// Package config reads service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full service configuration. All values come from the environment
// (see .env.example); secrets are never read from files in the repository.
type Config struct {
	HTTPAddr    string
	DatabaseURL string

	// BotToken is the MAX bot token. Empty token disables the bot and makes
	// initData validation impossible, so only DevMode requests are accepted.
	BotToken string

	// HMACSecret keys HMAC-SHA256 of phone numbers and personal account numbers.
	HMACSecret []byte

	// InitDataMaxAge limits how old a mini-app launch (auth_date) may be.
	InitDataMaxAge time.Duration

	// DevMode accepts requests without initData (X-Dev-User-Id header).
	// Must never be enabled on a server reachable by real users.
	DevMode bool

	// QuietHours holds poll invitations, poll results and question relays from 22:00
	// to 09:00 of the house (решение 29). false sends them at once: for testing in
	// the evening.
	QuietHours bool

	// SeedDemo creates the synthetic demo house on start if it does not exist.
	SeedDemo bool
	// DemoInviteSlug is the invite slug of the demo house. Empty value makes the
	// seed generate a random one and print it to the log.
	DemoInviteSlug string
	// UKMaxUserIDs grants the demo management organization to these MAX accounts.
	UKMaxUserIDs           []int64
	UKMaxUserIDsConfigured bool

	LogLevel string
}

// Load reads configuration from the environment and validates it.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:       env("HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		BotToken:       os.Getenv("MAX_BOT_TOKEN"),
		HMACSecret:     []byte(os.Getenv("HMAC_SECRET")),
		DemoInviteSlug: os.Getenv("DEMO_INVITE_SLUG"),
		LogLevel:       env("LOG_LEVEL", "info"),
	}

	var err error
	if c.DevMode, err = envBool("DEV_MODE", false); err != nil {
		return c, err
	}
	if c.QuietHours, err = envBool("QUIET_HOURS", true); err != nil {
		return c, err
	}
	if c.SeedDemo, err = envBool("SEED_DEMO", true); err != nil {
		return c, err
	}
	if c.InitDataMaxAge, err = time.ParseDuration(env("INITDATA_MAX_AGE", "24h")); err != nil {
		return c, fmt.Errorf("INITDATA_MAX_AGE: %w", err)
	}
	if c.InitDataMaxAge <= 0 {
		return c, fmt.Errorf("INITDATA_MAX_AGE must be positive, got %v", c.InitDataMaxAge)
	}
	if c.DemoInviteSlug != "" {
		for _, r := range c.DemoInviteSlug {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
			if !ok {
				return c, fmt.Errorf("DEMO_INVITE_SLUG contains invalid character %q; allowed: [A-Za-z0-9_-]", string(r))
			}
		}
	}
	rawUKIDs, ukConfigured := os.LookupEnv("UK_MAX_USER_IDS")
	c.UKMaxUserIDsConfigured = ukConfigured
	if raw := strings.TrimSpace(rawUKIDs); raw != "" {
		seen := make(map[int64]bool)
		for _, part := range strings.Split(raw, ",") {
			id, parseErr := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if parseErr != nil || id <= 0 {
				return c, fmt.Errorf("UK_MAX_USER_IDS: expected comma-separated positive MAX user ids")
			}
			if !seen[id] {
				c.UKMaxUserIDs = append(c.UKMaxUserIDs, id)
				seen[id] = true
			}
		}
	}

	var errs []error
	if c.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(c.HMACSecret) < 32 {
		errs = append(errs, errors.New("HMAC_SECRET must be at least 32 bytes"))
	}
	if c.BotToken == "" && !c.DevMode {
		errs = append(errs, errors.New("MAX_BOT_TOKEN is required unless DEV_MODE=true"))
	}

	return c, errors.Join(errs...)
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}

	return def
}

func envBool(key string, def bool) (bool, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}

	return b, nil
}
