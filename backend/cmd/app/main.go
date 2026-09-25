// Command app is the whole backend: HTTP API of the mini-app now, the bot and the
// background worker in the next steps (one binary, see docs/04 «Модули»).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	maxapi "github.com/max-messenger/max-bot-api-client-go/v2"
	"github.com/max-messenger/max-bot-api-client-go/v2/model"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/bot"
	"maxhackathon/backend/internal/platform/config"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/httpapi"
	"maxhackathon/backend/internal/platform/maxbot"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
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
	houses := registry.NewStore(pool)
	users := access.NewStore(pool)

	if cfg.SeedDemo {
		slug, err := houses.SeedDemo(ctx, hasher, cfg.DemoInviteSlug, log)
		if err != nil {
			return err
		}
		// The invite link is public by design (it hangs on the entrance door), logging it is fine.
		log.Info("demo house ready", "invite_slug", slug)
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
		Houses:   houses,
		Profiles: users,
		DB:       pool,
		Log:      log,
		DevMode:  cfg.DevMode,
	})

	var wg sync.WaitGroup
	if cfg.BotToken != "" {
		wg.Go(func() { runBot(ctx, cfg.BotToken, houses, log) })
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

// runBot starts the chat bot. Problems with the MAX API never stop the service:
// the HTTP API of the mini-app keeps working, the bot retries.
func runBot(ctx context.Context, token string, houses bot.Houses, log *slog.Logger, opts ...maxapi.Opt) {
	api, err := maxapi.NewApi(token, opts...)
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

	log.Info("bot started", "username", me.Username, "bot_id", me.UserID)
	poller := &maxbot.Poller{
		Updates: api.Subscriptions,
		Handler: &bot.Bot{
			Messages: api.Messages,
			Houses:   houses,
			Me:       bot.Identity{UserID: me.UserID, Username: me.Username},
			Log:      log,
		},
		Log: log,
	}
	poller.Run(ctx)
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
