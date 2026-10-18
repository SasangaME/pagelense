package fetcher

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{ip: "127.0.0.1", want: true},
		{ip: "127.255.255.254", want: true},
		{ip: "::1", want: true},
		{ip: "10.0.0.1", want: true},
		{ip: "172.16.5.4", want: true},
		{ip: "192.168.1.1", want: true},
		{ip: "169.254.169.254", want: true}, // cloud metadata
		{ip: "fd00:ec2::254", want: true},   // cloud metadata over IPv6
		{ip: "fe80::1", want: true},
		{ip: "0.0.0.0", want: true},
		{ip: "::", want: true},
		{ip: "100.100.100.200", want: true}, // carrier-grade NAT range
		{ip: "224.0.0.1", want: true},
		{ip: "255.255.255.255", want: true},
		{ip: "::ffff:127.0.0.1", want: true}, // IPv4-mapped loopback
		{ip: "::ffff:169.254.169.254", want: true},
		{ip: "64:ff9b::a00:1", want: true}, // NAT64 of 10.0.0.1
		{ip: "93.184.216.34", want: false},
		{ip: "8.8.8.8", want: false},
		{ip: "172.32.0.1", want: false}, // just outside 172.16.0.0/12
		{ip: "2606:4700:4700::1111", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := isBlocked(netip.MustParseAddr(tt.ip)); got != tt.want {
				t.Errorf("isBlocked(%s) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestSafeControl(t *testing.T) {
	if err := safeControl("tcp4", "93.184.216.34:443", nil); err != nil {
		t.Errorf("public address: %v", err)
	}
	if err := safeControl("tcp4", "169.254.169.254:80", nil); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("metadata address: error = %v, want ErrBlockedAddress", err)
	}
	if err := safeControl("tcp", "not-an-address", nil); !errors.Is(err, ErrBlockedAddress) {
		t.Errorf("unparsable address: error = %v, want ErrBlockedAddress", err)
	}
}

func TestFetchBlockedIP(t *testing.T) {
	var hit bool
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		hit = true
	}))
	defer srv.Close()

	opts := testOptions()
	opts.AllowPrivateIPs = false
	f := New(opts)

	// localhost resolves through the hosts file, so the check must run after DNS
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	for _, u := range []string{srv.URL, "http://localhost:" + port} {
		t.Run(u, func(t *testing.T) {
			_, err := f.Fetch(context.Background(), u)
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("error = %v, want ErrBlockedAddress", err)
			}
			fe := requireFetchError(t, err, http.StatusForbidden)
			if !strings.Contains(fe.Message, "private or reserved address") {
				t.Errorf("Message = %q", fe.Message)
			}
		})
	}
	if hit {
		t.Error("request reached the blocked server")
	}
}
