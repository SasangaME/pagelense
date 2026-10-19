package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestLoadWebDefaults(t *testing.T) {
	cfg, err := LoadWeb(env(nil))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}
	if cfg != DefaultWeb() {
		t.Errorf("got %+v, want defaults %+v", cfg, DefaultWeb())
	}
}

func TestLoadWebEmptyValuesUseDefaults(t *testing.T) {
	cfg, err := LoadWeb(env(map[string]string{"API_URL": "", "API_TIMEOUT": "", "PORT": ""}))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}
	if cfg != DefaultWeb() {
		t.Errorf("got %+v, want defaults %+v", cfg, DefaultWeb())
	}
}

func TestLoadWebOverrides(t *testing.T) {
	cfg, err := LoadWeb(env(map[string]string{
		"PORT":             "3000",
		"ADMIN_PORT":       "3001",
		"LOG_LEVEL":        "warn",
		"SHUTDOWN_TIMEOUT": "20s",
		"API_URL":          "http://api:8081",
		"API_TIMEOUT":      "90s",
	}))
	if err != nil {
		t.Fatalf("LoadWeb: %v", err)
	}
	want := WebConfig{
		Port:            3000,
		AdminPort:       3001,
		LogLevel:        slog.LevelWarn,
		ShutdownTimeout: 20 * time.Second,
		APIURL:          "http://api:8081",
		APITimeout:      90 * time.Second,
	}
	if cfg != want {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
}

func TestLoadWebInvalid(t *testing.T) {
	tests := []struct {
		key, value string
	}{
		{"PORT", "web"},
		{"ADMIN_PORT", "70000"},
		{"LOG_LEVEL", "trace"},
		{"SHUTDOWN_TIMEOUT", "0"},
		{"API_URL", "localhost:8081"}, // parsed as scheme "localhost"
		{"API_URL", "api:8081"},
		{"API_URL", "ftp://api:8081"},
		{"API_URL", "http://"},
		{"API_URL", "http://api:8081/%zz"},
		{"API_TIMEOUT", "70"}, // no unit
		{"API_TIMEOUT", "-1s"},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.value, func(t *testing.T) {
			_, err := LoadWeb(env(map[string]string{tt.key: tt.value}))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.key+": ") {
				t.Errorf("error %q does not name %s", err, tt.key)
			}
		})
	}
}

func TestLoadWebSamePorts(t *testing.T) {
	_, err := LoadWeb(env(map[string]string{"ADMIN_PORT": "8080"}))
	if err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("got %v, want port clash error", err)
	}
}

func TestLoadWebReportsAllErrors(t *testing.T) {
	cfg, err := LoadWeb(env(map[string]string{"API_URL": "nope", "API_TIMEOUT": "never"}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, key := range []string{"API_URL", "API_TIMEOUT"} {
		if !strings.Contains(err.Error(), key+": ") {
			t.Errorf("error %q does not name %s", err, key)
		}
	}
	if cfg != (WebConfig{}) {
		t.Errorf("got %+v, want zero WebConfig on error", cfg)
	}
}

// TestDefaultAPITimeoutOutlastsBackend keeps the frontend from giving up on an analysis the backend would still finish.
func TestDefaultAPITimeoutOutlastsBackend(t *testing.T) {
	be := Default()
	// the budget cancels checks still in flight, so link checks never take longer than it
	worstCase := be.FetchTimeout + be.LinkBudget

	if got := DefaultWeb().APITimeout; got <= worstCase {
		t.Errorf("API_TIMEOUT default %v must be longer than the backend's worst case %v", got, worstCase)
	}
}
