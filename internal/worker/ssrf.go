package worker

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"syscall"
	"time"

	"github.com/drkwv34/courier-outbox/internal/domain"
)

// ErrDestinationBlocked is returned when SSRF protection rejects a resolved IP.
var ErrDestinationBlocked = errors.New("ssrf: destination blocked")

const lookupTimeout = 2 * time.Second

// LookupHostIPs resolves host using the process resolver. Empty or failed
// answers are returned as-is so URLPolicy can fail closed.
func LookupHostIPs(host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(context.Background(), lookupTimeout)
	defer cancel()
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

func ssrfDialControl(_ string, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return ErrDestinationBlocked
	}
	if domain.DeniedIP(ip) {
		return ErrDestinationBlocked
	}
	return nil
}
