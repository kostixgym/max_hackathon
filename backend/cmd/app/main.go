// Command app is the whole backend: HTTP API of the mini-app now, the bot and the
// background worker in the next steps (one binary, see docs/04 «Модули»).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"maxhackathon/backend/internal/access"
	"maxhackathon/backend/internal/platform/config"
	"maxhackathon/backend/internal/platform/db"
	"maxhackathon/backend/internal/platform/httpapi"
	"maxhackathon/backend/internal/platform/security"
	"maxhackathon/backend/internal/registry"
	"maxhackathon/backend/migrations"
)

func main() {
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
		Houses:  houses,
		DB:      pool,
		Log:     log,
		DevMode: cfg.DevMode,
	})

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

	return srv.Shutdown(shutdownCtx)
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
