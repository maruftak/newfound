// Package notify delivers change alerts to webhooks (generic, Slack, or Discord).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/maruftak/reconsentry/internal/diff"
)

// Notifier delivers a batch of changes for a scope.
type Notifier interface {
	Notify(ctx context.Context, scope string, changes []diff.Change) error
}

// Webhook posts changes to a single webhook URL using the given format.
type Webhook struct {
	URL    string
	Format string // "generic" | "slack" | "discord"
	Client *http.Client
}

// NewWebhook builds a Webhook notifier. An empty format defaults to "generic".
func NewWebhook(url, format string) *Webhook {
	if format == "" {
		format = "generic"
	}
	return &Webhook{URL: url, Format: format, Client: &http.Client{Timeout: 15 * time.Second}}
}

// Notify posts the changes. It is a no-op when there are no changes.
func (w *Webhook) Notify(ctx context.Context, scope string, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	body, err := w.payload(scope, changes)
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.Client.Do(req)
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}

func (w *Webhook) payload(scope string, changes []diff.Change) ([]byte, error) {
	switch w.Format {
	case "slack":
		return json.Marshal(map[string]string{"text": RenderText(scope, changes)})
	case "discord":
		return json.Marshal(map[string]string{"content": RenderText(scope, changes)})
	default:
		return json.Marshal(map[string]any{
			"scope":   scope,
			"count":   len(changes),
			"changes": changes,
		})
	}
}

// RenderText builds a human-readable multi-line summary of the changes.
func RenderText(scope string, changes []diff.Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "reconsentry: %d change(s) on %s", len(changes), scope)
	for _, c := range changes {
		fmt.Fprintf(&b, "\n• [%s] %s — %s", c.Kind, c.Host, c.Detail)
	}
	return b.String()
}
