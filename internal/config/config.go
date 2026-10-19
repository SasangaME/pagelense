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
	ShutdownTimeout time.Duration // SHUTDOWN_TIMEOUT: graceful shutdown; keep below the ECS stop timeout (30s)
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
		ShutdownTimeout: 15 * time.Second,
		LinkWorkers:     10,
		MaxLinks:        100,
	}
}

// Load reads settings with getenv (usually os.Getenv), using Default for unset or empty variables and reporting every invalid one.
func Load(getenv func(string) string) (Config, error) {
	cfg := Default()
	var errs []error

	port := func(key string, dst *int) {
		if v := getenv(key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 65535 {
				errs = append(errs, fmt.Errorf("%s: %q is not a port (1-65535)", key, v))
				return
			}
			*dst = n
		}
	}
	positiveInt := func(key string, dst *int) {
		if v := getenv(key); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				errs = append(errs, fmt.Errorf("%s: %q is not a positive integer", key, v))
				return
			}
			*dst = n
		}
	}
	positiveDuration := func(key string, dst *time.Duration) {
		if v := getenv(key); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				errs = append(errs, fmt.Errorf("%s: %q is not a positive duration (e.g. 10s, 500ms)", key, v))
				return
			}
			*dst = d
		}
	}

	port("PORT", &cfg.Port)
	port("ADMIN_PORT", &cfg.AdminPort)
	if v := getenv("LOG_LEVEL"); v != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			errs = append(errs, fmt.Errorf("LOG_LEVEL: %q is not one of debug, info, warn, error", v))
		}
	}
	positiveDuration("FETCH_TIMEOUT", &cfg.FetchTimeout)
	positiveDuration("LINK_TIMEOUT", &cfg.LinkTimeout)
	positiveDuration("SHUTDOWN_TIMEOUT", &cfg.ShutdownTimeout)
	positiveInt("LINK_WORKERS", &cfg.LinkWorkers)
	positiveInt("MAX_LINKS", &cfg.MaxLinks)

	if cfg.Port == cfg.AdminPort {
		errs = append(errs, fmt.Errorf("PORT and ADMIN_PORT must differ (both %d)", cfg.Port))
	}

	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}
