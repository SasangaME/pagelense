// Package config loads application settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Config holds the application settings. The zero value is not valid; use Load.
type Config struct {
	Port            int           // PORT: public app server
	AdminPort       int           // ADMIN_PORT: /metrics and /debug/pprof, never exposed publicly
	LogLevel        slog.Level    // LOG_LEVEL: debug, info, warn or error
	FetchTimeout    time.Duration // FETCH_TIMEOUT: whole page fetch, including redirects and body
	LinkTimeout     time.Duration // LINK_TIMEOUT: per checked link
	LinkBudget      time.Duration // LINK_BUDGET: all link checks of one analysis; links not finished in time are not counted
	ShutdownTimeout time.Duration // SHUTDOWN_TIMEOUT: how long in-flight requests get to finish on Ctrl+C or SIGTERM
	LinkWorkers     int           // LINK_WORKERS: concurrent link checks per analysis
	MaxLinks        int           // MAX_LINKS: unique links checked per analysis
}

// Default returns the settings used for variables that are unset or empty.
func Default() Config {
	return Config{
		Port:            8080,
		AdminPort:       9090,
		LogLevel:        slog.LevelInfo,
		FetchTimeout:    10 * time.Second,
		LinkTimeout:     5 * time.Second,
		LinkBudget:      10 * time.Second,
		ShutdownTimeout: 15 * time.Second,
		LinkWorkers:     10,
		MaxLinks:        100,
	}
}

// Load reads the backend's settings with getenv (usually os.Getenv), using Default for unset or empty variables and reporting every invalid one.
func Load(getenv func(string) string) (Config, error) {
	cfg := Default()
	l := loader{getenv: getenv}

	l.port("PORT", &cfg.Port)
	l.port("ADMIN_PORT", &cfg.AdminPort)
	l.logLevel("LOG_LEVEL", &cfg.LogLevel)
	l.positiveDuration("FETCH_TIMEOUT", &cfg.FetchTimeout)
	l.positiveDuration("LINK_TIMEOUT", &cfg.LinkTimeout)
	l.positiveDuration("LINK_BUDGET", &cfg.LinkBudget)
	l.positiveDuration("SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout)
	l.positiveInt("LINK_WORKERS", &cfg.LinkWorkers)
	l.positiveInt("MAX_LINKS", &cfg.MaxLinks)
	l.differentPorts(cfg.Port, cfg.AdminPort)

	if err := l.err(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// loader reads variables into settings and collects every invalid one, so a bad config is reported in one go.
type loader struct {
	getenv func(string) string
	errs   []error
}

func (l *loader) port(key string, dst *int) {
	if v := l.getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not a port (1-65535)", key, v))
			return
		}
		*dst = n
	}
}

func (l *loader) positiveInt(key string, dst *int) {
	if v := l.getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not a positive integer", key, v))
			return
		}
		*dst = n
	}
}

func (l *loader) positiveDuration(key string, dst *time.Duration) {
	if v := l.getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not a positive duration (e.g. 10s, 500ms)", key, v))
			return
		}
		*dst = d
	}
}

func (l *loader) logLevel(key string, dst *slog.Level) {
	if v := l.getenv(key); v != "" {
		if err := dst.UnmarshalText([]byte(v)); err != nil {
			l.errs = append(l.errs, fmt.Errorf("%s: %q is not one of debug, info, warn, error", key, v))
		}
	}
}

func (l *loader) differentPorts(port, adminPort int) {
	if port == adminPort {
		l.errs = append(l.errs, fmt.Errorf("PORT and ADMIN_PORT must differ (both %d)", port))
	}
}

func (l *loader) err() error {
	if err := errors.Join(l.errs...); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return nil
}
