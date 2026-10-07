package domain

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestURLPolicy_Validate(t *testing.T) {
	t.Parallel()

	secure := URLPolicy{AllowHTTP: false, ProtectSSRF: true}
	httpOK := URLPolicy{AllowHTTP: true, ProtectSSRF: true}
	open := URLPolicy{AllowHTTP: true, ProtectSSRF: false}

	tests := []struct {
		name   string
		policy URLPolicy
		url    string
		want   bool
	}{
		{name: "https public host", policy: secure, url: "https://example.com/hooks", want: true},
		{name: "https public IP", policy: secure, url: "https://8.8.8.8/hooks", want: true},
		{name: "https missing path ok", policy: secure, url: "https://example.com", want: true},
		{name: "http rejected by default", policy: secure, url: "http://example.com/hooks", want: false},
		{name: "http allowed when configured", policy: httpOK, url: "http://example.com/hooks", want: true},
		{name: "userinfo rejected", policy: secure, url: "https://user:pass@example.com/hooks", want: false},
		{name: "ftp rejected", policy: secure, url: "ftp://example.com/hooks", want: false},
		{name: "javascript rejected", policy: secure, url: "javascript:alert(1)", want: false},
		{name: "relative rejected", policy: secure, url: "/hooks", want: false},
		{name: "empty rejected", policy: secure, url: "", want: false},
		{name: "blank rejected", policy: secure, url: "   ", want: false},
		{name: "no host", policy: secure, url: "https:///hooks", want: false},
		{name: "loopback ipv4 https", policy: secure, url: "https://127.0.0.1/hooks", want: false},
		{name: "loopback ipv4 http allowed scheme", policy: httpOK, url: "http://127.0.0.1/hooks", want: false},
		{name: "loopback 127.0.0.2", policy: httpOK, url: "http://127.0.0.2:9090/hooks", want: false},
		{name: "link-local metadata", policy: secure, url: "https://169.254.169.254/", want: false},
		{name: "private 10", policy: secure, url: "https://10.0.0.1/hooks", want: false},
		{name: "private 192.168", policy: secure, url: "https://192.168.1.1/hooks", want: false},
		{name: "private 172.16", policy: secure, url: "https://172.16.0.1/hooks", want: false},
		{name: "cgnat", policy: secure, url: "https://100.64.0.1/hooks", want: false},
		{name: "this network", policy: secure, url: "https://0.0.0.1/hooks", want: false},
		{name: "unspecified ipv4", policy: secure, url: "https://0.0.0.0/hooks", want: false},
		{name: "broadcast", policy: secure, url: "https://255.255.255.255/hooks", want: false},
		{name: "multicast ipv4", policy: secure, url: "https://224.0.0.1/hooks", want: false},
		{name: "loopback ipv6", policy: secure, url: "https://[::1]/hooks", want: false},
		{name: "unspecified ipv6", policy: secure, url: "https://[::]/hooks", want: false},
		{name: "mapped loopback", policy: secure, url: "https://[::ffff:127.0.0.1]/hooks", want: false},
		{name: "mapped private", policy: secure, url: "https://[::ffff:10.1.2.3]/hooks", want: false},
		{name: "mapped link-local", policy: secure, url: "https://[::ffff:169.254.169.254]/hooks", want: false},
		{name: "ula ipv6", policy: secure, url: "https://[fc00::1]/hooks", want: false},
		{name: "link-local ipv6", policy: secure, url: "https://[fe80::1]/hooks", want: false},
		{name: "multicast ipv6", policy: secure, url: "https://[ff02::1]/hooks", want: false},
		{name: "hostname not resolved", policy: secure, url: "https://localhost/hooks", want: true},
		{name: "protection off loopback", policy: open, url: "http://127.0.0.1:9090/hooks", want: true},
		{name: "protection off private", policy: open, url: "http://192.168.0.5/hooks", want: true},
		{name: "protection off still rejects ftp", policy: open, url: "ftp://127.0.0.1/hooks", want: false},
		{name: "protection off still rejects userinfo", policy: open, url: "http://u:p@127.0.0.1/hooks", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.policy.Validate(tt.url)
			if tt.want {
				if err != nil {
					t.Fatalf("Validate(%q) = %v, want accept", tt.url, err)
				}
				return
			}
			if !errors.Is(err, ErrValidation) {
				t.Fatalf("Validate(%q) = %v, want ErrValidation", tt.url, err)
			}
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != "target_url" {
				t.Fatalf("Validate(%q) = %v, want *ValidationError on target_url", tt.url, err)
			}
			if ve != nil && (strings.Contains(ve.Reason, "user:pass") || (tt.url != "" && strings.Contains(ve.Reason, tt.url))) {
				t.Fatalf("error leaked URL or credentials: %v", err)
			}
		})
	}
}

func TestDeniedIP_Unmap(t *testing.T) {
	t.Parallel()

	loopback, err := netip.ParseAddr("::ffff:127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !DeniedIP(loopback) {
		t.Fatal("mapped loopback should be denied")
	}
	public, err := netip.ParseAddr("8.8.8.8")
	if err != nil {
		t.Fatal(err)
	}
	if DeniedIP(public) {
		t.Fatal("8.8.8.8 should be allowed")
	}
}
