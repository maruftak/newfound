package collect

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/maruftak/reconsentry/internal/model"
)

// certPort is the TLS port probed for certificate metadata.
const certPort = "443"

// tlsDial establishes a TLS connection and returns the leaf certificate's
// expiry. It is a package var so tests can substitute a fake without a real
// network. The dial respects the context deadline.
var tlsDial = func(ctx context.Context, host string) (time.Time, error) {
	d := &tls.Dialer{Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, certPort))
	if err != nil {
		return time.Time{}, err
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return time.Time{}, fmt.Errorf("no peer certificate")
	}
	return state.PeerCertificates[0].NotAfter, nil
}

// CertExpiry fetches the leaf-certificate expiry for each host over TLS on port
// 443. It performs active connections, so the runner only invokes it on
// non-passive scopes. A failure for one host (no TLS, closed port, handshake
// error) is skipped rather than failing the batch; an error is returned only
// when every host fails.
func CertExpiry(ctx context.Context, hosts []string) ([]model.CertInfo, error) {
	var infos []model.CertInfo
	var lastErr error
	failures := 0
	for _, h := range hosts {
		expiry, err := tlsDial(ctx, h)
		if err != nil {
			lastErr = err
			failures++
			continue
		}
		infos = append(infos, model.CertInfo{Host: h, Expiry: expiry})
	}
	if len(infos) == 0 && failures > 0 {
		return nil, fmt.Errorf("cert check: all %d host(s) failed: %w", failures, lastErr)
	}
	return infos, nil
}
