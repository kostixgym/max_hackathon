// Command app is the whole backend: HTTP API of the mini-app now, the bot and the
// background worker in the next steps (one binary, see docs/04 «Модули»).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
	// Houses keep local time zones (quiet hours, poll deadlines): the zone database
	// is built in, so it does not depend on the runtime image.
	_ "time/tzdata"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/bot"
	"maxhackathon/backend/internal/demand"
	"maxhackathon/backend/internal/documents"
	"maxhackathon/backend/internal/initiatives"
	"maxhackathon/backend/internal/meeting"
	"maxhackathon/backend/internal/notify"
	"maxhackathon/backend/internal/platform/config"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/httpapi"
	"maxhackathon/backend/internal/platform/maxbot"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/poll"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/internal/rules"
	"maxhackathon/backend/migrations"
)

func main() {
	// `app healthcheck` is the container health check: the runtime image
	// (distroless) has no shell and no curl.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := newLogger(cfg.LogLevel)

	if cfg.DevMode {
		log.Warn("DEV_MODE is on: requests without initData are accepted. Never enable it where real users are")
	}
	if cfg.BotToken == "" {
		log.Warn("MAX_BOT_TOKEN is empty: the bot is off and initData cannot be verified")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool, migrations.FS, log); err != nil {
		return err
	}

	hasher := security.NewHasher(cfg.HMACSecret)
	// Modules read each other's data only through interfaces; the wiring is here.
	// access needs the legacy initiatives reader, initiatives.Service needs access
	// as the poll audience: interfaces on both sides keep the wiring acyclic.
	houses := registry.NewStore(pool)
	tm := db.NewTransactionManager(pool)
	notifier := notify.NewStore(pool)
	catalog := rules.NewCatalog(pool)
	users := access.NewStore(pool, houses, initiatives.NewStore(pool), hasher)
	initService := initiatives.NewService(pool, tm, catalog, houses, users, notifier)
	initService.SetQuietHours(cfg.QuietHours)
	if !cfg.QuietHours {
		log.Warn("QUIET_HOURS is off: poll invitations and questions go out at night too")
	}
	polls := poll.NewStore(pool, initService, users, houses)
	meetings := meeting.NewService(pool, tm, initService, users, houses, notifier)
	demands := demand.NewService(pool, tm, initService, polls, users, meetings, houses, users, documents.DemandPDF, notifier)

	// Decision types and templates are platform data, not demo data (решение 9):
	// without them no initiative can be created, so they are seeded on every start.
	if err := catalog.SeedCatalog(ctx); err != nil {
		return fmt.Errorf("seed rules catalog: %w", err)
	}
	if cfg.SeedDemo {
		slug, err := houses.SeedDemo(ctx, hasher, cfg.DemoInviteSlug, log)
		if err != nil {
			return err
		}
		// The invite link is public by design (it hangs on the entrance door), logging it is fine.
		log.Info("demo house ready", "invite_slug", slug)
		if err := houses.SeedSampleHouses(ctx, hasher); err != nil {
			return err
		}
	}
	if cfg.UKMaxUserIDsConfigured {
		orgID, err := houses.DemoOrgID(ctx)
		if err != nil {
			return err
		}
		if err := users.ConfigureUK(ctx, orgID, cfg.UKMaxUserIDs, true); err != nil {
			return err
		}
	}

	handler := httpapi.NewHandler(httpapi.Deps{
		Auth: &httpapi.Authenticator{
			BotToken: cfg.BotToken,
			MaxAge:   cfg.InitDataMaxAge,
			DevMode:  cfg.DevMode,
			Users:    users,
			Log:      log,
			Now:      time.Now,
		},
		Houses:          houses,
		Profiles:        users,
		DB:              pool,
		Log:             log,
		DevMode:         cfg.DevMode,
		UKIDsConfigured: cfg.UKMaxUserIDsConfigured,

		Access:           users,
		Templates:        catalog,
		Initiatives:      initService,
		InitiativeReader: initService,
		PathSelector:     initService,
		PollStarter:      initService,
		PollProgress:     polls,
		Votes:            polls,
		DemoMembers:      users,
		GuestAttacher:    users,
		PhoneVerifier:    users,

		// Sprint to 30.09: the staff cabinet and the meeting.
		Orgs:      users,
		OrgHouses: houses,
		Meetings:  meetings,
		Demands:   demands,
	})

	var wg sync.WaitGroup
	if cfg.BotToken != "" {
		wg.Go(func() {
			runBot(ctx, cfg.BotToken, botDeps{
				houses:           houses,
				houseByID:        houses,
				users:            users,
				votes:            polls,
				pollVoteReader:   polls,
				progress:         polls,
				initiatives:      initService,
				initiativeReader: initService,
				questions:        initService,
				members:          users,
				accounts:         users,
				notifier:         notifier,
				devMode:          cfg.DevMode,
				ukIDs:            cfg.UKMaxUserIDs,
				demandsAnnounce:  demands,
				staff:            users,
				meetingsView:     meetings,
				protocol:         meetings,
				owners:           users,
			}, log)
		})
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("http server started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err = srv.Shutdown(shutdownCtx)
	wg.Wait()

	return err
}

// botDeps bundles the domain modules the bot and the worker talk to. The ones the
// end-to-end test does not need may be nil: their features then stay off.
type botDeps struct {
	houses           bot.Houses
	houseByID        bot.HouseReader
	users            bot.Users
	votes            bot.Votes
	pollVoteReader   bot.PollVotes
	progress         bot.PollProgress
	initiatives      bot.PollingReader
	initiativeReader bot.InitiativeReader
	questions        bot.Questions
	members          bot.Members
	accounts         bot.Accounts
	notifier         botNotifier
	devMode          bool
	ukIDs            []int64

	// К3: уведомления о требовании и собрании.
	demandsAnnounce bot.DemandAnnouncer
	staff           bot.StaffNotifiees
	meetingsView    bot.MeetingBotView
	protocol        bot.ProtocolReader
	owners          bot.OwnerNotifiees
}

// botNotifier is everything the bot runtime needs from the notify module:
// the job queue for the worker and markers for the poller.
type botNotifier interface {
	notify.Queue
	maxbot.Markers
}

// runBot starts the chat bot and the notify worker. Problems with the MAX API
// never stop the service: the HTTP API of the mini-app keeps working, the bot retries.
func runBot(ctx context.Context, token string, deps botDeps, log *slog.Logger, opts ...maxapi.Opt) {
	// The MAX API certificate chains to the Russian Trusted Root CA (see maxbot.HTTPClient).
	httpClient, err := maxbot.HTTPClient()
	if err != nil {
		log.Error("bot: HTTP client for MAX", "err", err)

		return
	}
	api, err := maxapi.NewApi(token, append([]maxapi.Opt{maxapi.WithHTTPClient(httpClient)}, opts...)...)
	if err != nil {
		log.Error("bot: create MAX client", "err", err)

		return
	}

	var me model.BotInfo
	for {
		if me, err = api.Bots.GetMyInfo(ctx); err == nil {
			break
		}
		log.Warn("bot: get bot info failed, retrying", "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}

	// Long polling does not receive updates while a webhook subscription exists.
	if subs, err := api.Subscriptions.GetSubscriptions(ctx); err == nil && len(subs.Subscriptions) > 0 {
		log.Warn("bot: webhook subscriptions exist, long polling will get no updates", "count", len(subs.Subscriptions))
	}

	identity := bot.Identity{UserID: me.UserID, Username: me.Username}
	inviter := &bot.PollInviter{
		Messages:    api.Messages,
		Initiatives: deps.initiatives,
		Houses:      deps.houseByID,
		Progress:    deps.progress,
		Me:          identity,
	}
	reminder := &bot.Reminder{Inviter: inviter, Votes: deps.pollVoteReader}
	result := &bot.PollResult{
		Messages:    api.Messages,
		Initiatives: deps.initiativeReader,
		Houses:      deps.houseByID,
		Progress:    deps.progress,
		Accounts:    deps.accounts,
		Me:          identity,
	}
	relay := &bot.QuestionRelay{
		Messages:    api.Messages,
		Questions:   deps.questions,
		Initiatives: deps.initiativeReader,
		Houses:      deps.houseByID,
		Accounts:    deps.accounts,
		Me:          identity,
	}
	demandDelivered := &bot.DemandDeliveredNotifier{
		Messages: api.Messages,
		Announce: deps.demandsAnnounce,
		Staff:    deps.staff,
		Log:      log,
	}
	meetingCreated := &bot.MeetingCreatedNotifier{
		Messages: api.Messages,
		View:     deps.meetingsView,
		Owners:   deps.owners,
		Log:      log,
	}
	meetingFinalized := &bot.MeetingFinalizedNotifier{
		Messages: api.Messages,
		View:     deps.meetingsView,
		Protocol: deps.protocol,
		Owners:   deps.owners,
		Log:      log,
	}
	var wg sync.WaitGroup

	wg.Go(func() {
		worker := &notify.Worker{
			Queue: deps.notifier,
			Handlers: map[string]notify.JobHandler{
				notify.TypePollInvite:       inviter.HandleJob,
				notify.TypePollReminder:     reminder.HandleJob,
				notify.TypePollFinished:     result.HandleJob,
				notify.TypeQuestionAsked:    relay.HandleAsked,
				notify.TypeQuestionAnswered: relay.HandleAnswered,
				notify.TypeDemandDelivered:  demandDelivered.HandleJob,
				notify.TypeMeetingCreated:   meetingCreated.HandleJob,
				notify.TypeMeetingFinalized: meetingFinalized.HandleJob,
			},
			Log: log,
		}
		worker.Run(ctx)
	})

	log.Info("bot started", "username", me.Username, "bot_id", me.UserID)
	poller := &maxbot.Poller{
		Updates: api.Subscriptions,
		Handler: &bot.Bot{
			Messages:         api.Messages,
			Houses:           deps.houses,
			Me:               identity,
			Log:              log,
			Users:            deps.users,
			Votes:            deps.votes,
			Answers:          api.Messages,
			Initiatives:      deps.initiatives,
			HousesByID:       deps.houseByID,
			Progress:         deps.progress,
			Questions:        deps.questions,
			Members:          deps.members,
			InitiativeReader: deps.initiativeReader,
			DevMode:          deps.devMode,
			UKIDs:            deps.ukIDs,
		},
		Log:     log,
		BotID:   me.UserID,
		Markers: deps.notifier,
	}
	poller.Run(ctx)
	wg.Wait()
	log.Info("bot stopped")
}

// healthcheck asks the running service whether it is ready (PostgreSQL reachable).
func healthcheck() int {
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 1
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/api/v1/readyz")
	if err != nil {
		return 1
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
