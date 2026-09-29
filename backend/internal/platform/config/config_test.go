package config

import (
	"reflect"
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

func TestUKMaxUserIDs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/app")
	t.Setenv("HMAC_SECRET", strings.Repeat("s", 32))
	t.Setenv("DEV_MODE", "true")
	t.Setenv("UK_MAX_USER_IDS", "42, 43,42")
	cfg, err := Load()
	if err != nil || !cfg.UKMaxUserIDsConfigured || !reflect.DeepEqual(cfg.UKMaxUserIDs, []int64{42, 43}) {
		t.Fatalf("UK IDs = %v, configured = %v, err = %v", cfg.UKMaxUserIDs, cfg.UKMaxUserIDsConfigured, err)
	}
	t.Setenv("UK_MAX_USER_IDS", "42,abc")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "UK_MAX_USER_IDS") {
		t.Fatalf("invalid list: %v", err)
	}
}
