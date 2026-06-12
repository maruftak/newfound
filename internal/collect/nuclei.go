package collect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/maruftak/reconsentry/internal/model"
)

// nucleiLine models the subset of `nuclei -jsonl` output we consume.
type nucleiLine struct {
	TemplateID string `json:"template-id"`
	Host       string `json:"host"`
	MatchedAt  string `json:"matched-at"`
	URL        string `json:"url"`
	Info       struct {
		Name     string `json:"name"`
		Severity string `json:"severity"`
	} `json:"info"`
}

// Nuclei scans the given hosts with the nuclei CLI and returns findings. It is
// used by `run --scan-new` to probe newly-discovered hosts for known issues.
func Nuclei(ctx context.Context, hosts []string) ([]model.Finding, error) {
	if len(hosts) == 0 {
		return nil, nil
	}
	if err := ensure("nuclei"); err != nil {
		return nil, err
	}
	args := []string{"-silent", "-jsonl", "-disable-update-check"}
	out, err := runStdin(ctx, strings.Join(hosts, "\n"), "nuclei", args...)
	if err != nil {
		return nil, err
	}
	return parseNuclei(out), nil
}

// parseNuclei converts nuclei JSONL output into findings.
func parseNuclei(b []byte) []model.Finding {
	var findings []model.Finding
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var l nucleiLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			continue
		}
		url := l.MatchedAt
		if url == "" {
			url = l.URL
		}
		host := l.Host
		if host == "" {
			host = cleanHost(url)
		}
		findings = append(findings, model.Finding{
			Host:       host,
			TemplateID: l.TemplateID,
			Name:       l.Info.Name,
			Severity:   l.Info.Severity,
			URL:        url,
		})
	}
	return findings
}
