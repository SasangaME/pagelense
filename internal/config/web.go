package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"time"
)

// WebConfig holds the frontend's settings. The zero value is not valid; use LoadWeb.
type WebConfig struct {
	Port            int           // PORT: public web server
	AdminPort       int           // ADMIN_PORT: /metrics and /debug/pprof, never exposed publicly
	LogLevel        slog.Level    // LOG_LEVEL: debug, info, warn or error
	ShutdownTimeout time.Duration // SHUTDOWN_TIMEOUT: how long in-flight requests get to finish on Ctrl+C or SIGTERM
	APIURL          string        // API_URL: backend base URL, e.g. http://localhost:8081
	APITimeout      time.Duration // API_TIMEOUT: whole backend call; must outlast the backend's fetch plus link checks
}

// DefaultWeb returns the frontend settings used for variables that are unset or empty.
func DefaultWeb() WebConfig {
	return WebConfig{
		Port:            8080,
		AdminPort:       9090,
		LogLevel:        slog.LevelInfo,
		ShutdownTimeout: 15 * time.Second,
		APIURL:          "http://localhost:8081",
		// the backend's defaults allow 10s fetch + 10s link budget = 20s, plus headroom
		APITimeout: 30 * time.Second,
	}
}

// LoadWeb reads the frontend's settings with getenv (usually os.Getenv), using DefaultWeb for unset or empty variables and reporting every invalid one.
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

// baseURL accepts an absolute http(s) URL with a host, the same rule the backend client applies.
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
