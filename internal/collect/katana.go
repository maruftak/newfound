package collect

import (
	"bufio"
	"bytes"
	"context"
	"strings"

	"github.com/maruftak/reconsentry/internal/model"
)

// Katana crawls the given live hosts with the katana CLI and returns the URLs
// it finds (endpoints + params). Used by `run --crawl`.
func Katana(ctx context.Context, hosts []string) ([]model.Endpoint, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	if err := ensure("katana"); err != nil {
		return nil, err
	}
	targets := make([]string, 0, len(hosts))
	for _, h := range hosts {
		targets = append(targets, "https://"+h)
	}
	args := []string{"-silent", "-d", "2"}
	out, err := runStdin(ctx, strings.Join(targets, "\n"), "katana", args...)
	if err != nil {
		return nil, err
	}
	return parseKatana(out), nil
}

// parseKatana turns katana's newline-delimited URL output into endpoints,
// de-duplicated and with fragments stripped.
func parseKatana(b []byte) []model.Endpoint {
	seen := map[string]bool{}
	var eps []model.Endpoint
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		u := strings.TrimSpace(sc.Text())
		if i := strings.IndexByte(u, '#'); i >= 0 {
			u = u[:i]
		}
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		eps = append(eps, model.Endpoint{URL: u, Host: cleanHost(u)})
	}
	return eps
}
