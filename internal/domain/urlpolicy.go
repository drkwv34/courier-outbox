package domain

import (
	"net/netip"
	"net/url"
	"strings"
)

// LookupIPs resolves a hostname. It is injected so this package stays
// free of resolver I/O. A nil lookup fails closed when ProtectSSRF is true.
type LookupIPs func(host string) ([]netip.Addr, error)

// URLPolicy is the create/update check for subscription target URLs.
type URLPolicy struct {
	AllowHTTP   bool
	ProtectSSRF bool
	Lookup      LookupIPs
}

// deniedPrefixes is the IP deny-list (NFR-SEC-002). Hostnames are resolved
// through Lookup at create/update; the worker also checks the dialed IP.
var deniedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("255.255.255.255/32"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// DeniedIP reports whether addr is in a blocked range. IPv4-mapped IPv6
// addresses are unmapped first.
func DeniedIP(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	addr = addr.Unmap()
	for _, p := range deniedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// Validate parses raw as an absolute http(s) URL and applies scheme, userinfo,
// host, and (when ProtectSSRF) IP-literal and hostname-resolution checks.
// The raw value is never echoed in the error.
func (p URLPolicy) Validate(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return &ValidationError{Field: "target_url", Reason: "is required"}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || !u.IsAbs() {
		return &ValidationError{Field: "target_url", Reason: "must be an absolute URL"}
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !p.AllowHTTP {
			return &ValidationError{Field: "target_url", Reason: "must use https"}
		}
	default:
		if p.AllowHTTP {
			return &ValidationError{Field: "target_url", Reason: "scheme must be http or https"}
		}
		return &ValidationError{Field: "target_url", Reason: "scheme must be https"}
	}
	if u.User != nil {
		return &ValidationError{Field: "target_url", Reason: "must not contain userinfo"}
	}
	host := u.Hostname()
	if host == "" {
		return &ValidationError{Field: "target_url", Reason: "has no host"}
	}
	if !p.ProtectSSRF {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return p.checkResolved(host)
	}
	if DeniedIP(ip) {
		return &ValidationError{Field: "target_url", Reason: "destination is blocked"}
	}
	return nil
}

func (p URLPolicy) checkResolved(host string) error {
	if p.Lookup == nil {
		return &ValidationError{Field: "target_url", Reason: "destination is blocked"}
	}
	addrs, err := p.Lookup(host)
	if err != nil || len(addrs) == 0 {
		return &ValidationError{Field: "target_url", Reason: "destination is blocked"}
	}
	for _, a := range addrs {
		if DeniedIP(a) {
			return &ValidationError{Field: "target_url", Reason: "destination is blocked"}
		}
	}
	return nil
}
