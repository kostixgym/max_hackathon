package config

import (
	"strings"
	"testing"
)

func TestQuietHours(t *testing.T) {
	tests := []struct {
		value   string
		want    bool
		wantErr bool
	}{
		{value: "", want: true}, // on by default (решение 29)
		{value: "false", want: false},
		{value: "true", want: true},
		{value: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("QUIET_HOURS="+tt.value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://localhost/app")
			t.Setenv("HMAC_SECRET", strings.Repeat("s", 32))
			t.Setenv("DEV_MODE", "true")
			t.Setenv("QUIET_HOURS", tt.value)

			cfg, err := Load()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "QUIET_HOURS") {
					t.Fatalf("err = %v, want a QUIET_HOURS error", err)
				}

				return
			}
			if err != nil || cfg.QuietHours != tt.want {
				t.Fatalf("QuietHours = %v, %v; want %v", cfg.QuietHours, err, tt.want)
			}
		})
	}
}

func TestLegacyUKMaxUserIDsIgnored(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/app")
	t.Setenv("HMAC_SECRET", strings.Repeat("s", 32))
	t.Setenv("DEV_MODE", "true")
	t.Setenv("UK_MAX_USER_IDS", "42,not-a-number")
	if _, err := Load(); err != nil {
		t.Fatalf("legacy variable should be ignored: %v", err)
	}
}
