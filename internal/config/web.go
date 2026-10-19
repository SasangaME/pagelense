package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"
)

// WebConfig holds the frontend's settings.
type WebConfig struct {
	Port            int           // PORT
	AdminPort       int           // ADMIN_PORT
	LogLevel        slog.Level    // LOG_LEVEL
	ShutdownTimeout time.Duration // SHUTDOWN_TIMEOUT
	APIURL          string        // API_URL
	APITimeout      time.Duration // API_TIMEOUT
}

// DefaultWeb returns the frontend defaults.
func DefaultWeb() WebConfig {
	return WebConfig{
		Port:            8080,
		AdminPort:       9090,
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: 15 * time.Second,
		APIURL:          "http://localhost:8081",
		// backend worst case: 10s fetch + 10s link budget
		APITimeout: 30 * time.Second,
	}
}

// LoadWeb reads the frontend's settings from the environment.
func LoadWeb(getenv func(string) string) (WebConfig, error) {
	cfg := DefaultWeb()
	l := loader{getenv: getenv}

	l.port("PORT", &cfg.Port)
	l.port("ADMIN_PORT", &cfg.AdminPort)
	l.logLevel("LOG_LEVEL", &cfg.LogLevel)
	l.positiveDuration("SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout)
	l.baseURL("API_URL", &cfg.APIURL)
	l.positiveDuration("API_TIMEOUT", &cfg.APITimeout)
	l.differentPorts(cfg.Port, cfg.AdminPort)

	if err := l.err(); err != nil {
		return WebConfig{}, err
	}
	return cfg, nil
}

func (l *loader) baseURL(key string, dst *string) {
	if v := l.getenv(key); v != "" {
		u, err := url.Parse(v)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not an absolute http:// or https:// URL", key, v))
			return
		}
		*dst = v
	}
}
