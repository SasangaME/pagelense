package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func env(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg != Default() {
		t.Errorf("got %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadEmptyValuesUseDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"PORT": "", "LOG_LEVEL": "", "FETCH_TIMEOUT": ""}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg != Default() {
		t.Errorf("got %+v, want defaults %+v", cfg, Default())
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"PORT":             "3000",
		"ADMIN_PORT":       "3001",
		"LOG_LEVEL":        "DEBUG",
		"FETCH_TIMEOUT":    "20s",
		"LINK_TIMEOUT":     "1500ms",
		"LINK_BUDGET":      "45s",
		"SHUTDOWN_TIMEOUT": "25s",
		"LINK_WORKERS":     "32",
		"MAX_LINKS":        "500",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Port:            3000,
		AdminPort:       3001,
		LogLevel:        slog.LevelDebug,
		FetchTimeout:    20 * time.Second,
		LinkTimeout:     1500 * time.Millisecond,
		LinkBudget:      45 * time.Second,
		ShutdownTimeout: 25 * time.Second,
		LinkWorkers:     32,
		MaxLinks:        500,
	}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadInvalid(t *testing.T) {
	tests := []struct {
		key, value string
	}{
		{"PORT", "abc"},
		{"PORT", "0"},
		{"PORT", "65536"},
		{"ADMIN_PORT", "-1"},
		{"LOG_LEVEL", "verbose"},
		{"FETCH_TIMEOUT", "10"}, // no unit
		{"FETCH_TIMEOUT", "0s"},
		{"LINK_TIMEOUT", "-5s"},
		{"LINK_BUDGET", "0s"},
		{"SHUTDOWN_TIMEOUT", "soon"},
		{"LINK_WORKERS", "0"},
		{"LINK_WORKERS", "1.5"},
		{"MAX_LINKS", "-10"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			_, err := Load(env(map[string]string{tt.key: tt.value}))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.key+": ") {
				t.Errorf("error %q does not name %s", err, tt.key)
			}
		})
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	cfg, err := Load(env(map[string]string{"PORT": "x", "LINK_WORKERS": "y", "MAX_LINKS": "z"}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"PORT", "LINK_WORKERS", "MAX_LINKS"} {
		if !strings.Contains(err.Error(), key+": ") {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
	if cfg != (Config{}) {
		t.Errorf("got %+v, want zero Config on error", cfg)
	}
}
