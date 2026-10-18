package analyzer

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ErrInvalidURL is returned when the input is not an absolute http or https URL.
var ErrInvalidURL = errors.New("invalid URL")

// parseURL validates the user's input and returns it with a lowercase host and no fragment.
func parseURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("%w: URL is required", ErrInvalidURL)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("%w: must start with http:// or https://", ErrInvalidURL)
	}
	if u.Hostname() == "" {
		return nil, fmt.Errorf("%w: missing host", ErrInvalidURL)
	}

	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	return u, nil
}
