package fetcher

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// ErrBlockedAddress is returned when a host resolves to a non-public IP.
var ErrBlockedAddress = errors.New("address not allowed")

func isBlocked(ip netip.Addr) bool {
	ip = ip.Unmap()
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// safeControl runs after DNS resolution, so redirects and DNS rebinding are covered too.
func safeControl(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, address)
	}
	if isBlocked(ap.Addr()) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, ap.Addr())
	}
	return nil
}

func newTransport(allowPrivate bool) *http.Transport {
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	if !allowPrivate {
		dialer.Control = safeControl
	}
	// no Proxy: it would dial for us and skip the check
	return &http.Transport{
		DialContext:       dialer.DialContext,
		ForceAttemptHTTP2: true,
		MaxIdleConns:      100,
		// most link checks go to the page's own host
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}
