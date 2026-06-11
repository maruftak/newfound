// Package notify delivers change alerts to webhooks (generic, Slack, or Discord).
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maruftak/reconsentry/internal/diff"
)

// maxAttempts bounds delivery retries for transient failures.
const maxAttempts = 3

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

// Notify posts the changes, retrying transient failures with backoff. It is a
// no-op when there are no changes.
func (w *Webhook) Notify(ctx context.Context, scope string, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	body, err := w.payload(scope, changes)
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}
	return deliver(ctx, w.Client, w.URL, body)
}

// deliver posts a JSON body to url with bounded retries on transient failures.
func deliver(ctx context.Context, client *http.Client, url string, body []byte) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		lastErr = postOnce(ctx, client, url, body)
		if lastErr == nil {
			return nil
		}
		if !retryable(lastErr) || attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt) * 500 * time.Millisecond):
		}
	}
	return lastErr
}

// postOnce performs a single JSON POST.
func postOnce(ctx context.Context, client *http.Client, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused
	if resp.StatusCode >= 300 {
		return &httpError{status: resp.StatusCode}
	}
	return nil
}

// httpError carries a non-2xx response status so retryable can classify it.
type httpError struct{ status int }

func (e *httpError) Error() string { return fmt.Sprintf("webhook returned status %d", e.status) }

// retryable reports whether err is worth another attempt: transport errors and
// 429/5xx responses are; 4xx (bad URL, auth) are not.
func retryable(err error) bool {
	var he *httpError
	if errors.As(err, &he) {
		return he.status == http.StatusTooManyRequests || he.status >= 500
	}
	return true
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

// Telegram delivers alerts via the Telegram Bot API (sendMessage).
type Telegram struct {
	Token   string
	ChatID  string
	BaseURL string // defaults to https://api.telegram.org; overridable for tests
	Client  *http.Client
}

// NewTelegram builds a Telegram notifier for a bot token and chat id.
func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{
		Token:   token,
		ChatID:  chatID,
		BaseURL: "https://api.telegram.org",
		Client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// Notify sends the changes as a Telegram message. It is a no-op when there are
// no changes.
func (t *Telegram) Notify(ctx context.Context, scope string, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	body, err := json.Marshal(map[string]string{
		"chat_id": t.ChatID,
		"text":    RenderText(scope, changes),
	})
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}
	base := t.BaseURL
	if base == "" {
		base = "https://api.telegram.org"
	}
	return deliver(ctx, t.Client, base+"/bot"+t.Token+"/sendMessage", body)
}

// RenderText builds a human-readable multi-line summary of the changes,
// appending a clickable URL per host so an alert is actionable at a glance.
func RenderText(scope string, changes []diff.Change) string {
	var b strings.Builder
	fmt.Fprintf(&b, "reconsentry: %d change(s) on %s", len(changes), scope)
	for _, c := range changes {
		fmt.Fprintf(&b, "\n• [%s] %s — %s", c.Kind, c.Host, c.Detail)
		if c.Host != "" {
			fmt.Fprintf(&b, " (https://%s)", c.Host)
		}
	}
	return b.String()
}
