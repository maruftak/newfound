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
	"net/smtp"
	neturl "net/url"
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
		// Drop the URL from transport errors: it can embed a secret — a Telegram
		// bot token in the path, or a Slack/Discord webhook URL that is itself a
		// credential. The caller already knows which destination it targeted.
		var ue *neturl.Error
		if errors.As(err, &ue) {
			return fmt.Errorf("post failed: %w", ue.Err)
		}
		return fmt.Errorf("post failed: %w", err)
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
		return json.Marshal(renderSlack(scope, changes))
	case "discord":
		return json.Marshal(renderDiscord(scope, changes))
	default:
		return json.Marshal(map[string]any{
			"scope":   scope,
			"count":   len(changes),
			"changes": changes,
		})
	}
}

func renderSlack(scope string, changes []diff.Change) map[string]any {
	return map[string]any{
		"text": RenderText(scope, changes),
		"attachments": []map[string]any{{
			"color": priorityColor(maxPriority(changes)),
			"blocks": []map[string]any{
				{
					"type": "header",
					"text": map[string]string{
						"type": "plain_text",
						"text": fmt.Sprintf("reconsentry: %d change(s) on %s", len(changes), scope),
					},
				},
				{
					"type": "section",
					"text": map[string]string{
						"type": "mrkdwn",
						"text": slackChangeList(changes),
					},
				},
			},
		}},
	}
}

func renderDiscord(scope string, changes []diff.Change) map[string]any {
	fields := make([]map[string]any, 0, len(changes))
	for _, c := range changes {
		fields = append(fields, map[string]any{
			"name":   fmt.Sprintf("%s · %s", c.Kind, priorityName(c.Priority)),
			"value":  discordChange(c),
			"inline": false,
		})
	}
	return map[string]any{
		"content": RenderText(scope, changes),
		"embeds": []map[string]any{{
			"title":  fmt.Sprintf("reconsentry: %d change(s) on %s", len(changes), scope),
			"color":  priorityColorInt(maxPriority(changes)),
			"fields": fields,
		}},
	}
}

func slackChangeList(changes []diff.Change) string {
	var b strings.Builder
	for i, c := range changes {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "• *%s* · %s · %s — %s", c.Kind, priorityName(c.Priority), slackHost(c.Host), c.Detail)
	}
	return b.String()
}

func discordChange(c diff.Change) string {
	if c.Host == "" {
		return c.Detail
	}
	return fmt.Sprintf("%s — %s", discordHost(c.Host), c.Detail)
}

func slackHost(host string) string {
	if host == "" {
		return "unknown host"
	}
	return fmt.Sprintf("<https://%s|%s>", host, host)
}

func discordHost(host string) string {
	if host == "" {
		return "unknown host"
	}
	return fmt.Sprintf("[%s](https://%s)", host, host)
}

func maxPriority(changes []diff.Change) int {
	max := diff.Low
	for _, c := range changes {
		if c.Priority > max {
			max = c.Priority
		}
	}
	return max
}

func priorityName(p int) string {
	switch p {
	case diff.High:
		return "high"
	case diff.Medium:
		return "medium"
	default:
		return "low"
	}
}

func priorityColor(p int) string {
	switch p {
	case diff.High:
		return "#d73a49"
	case diff.Medium:
		return "#d29922"
	default:
		return "#57606a"
	}
}

func priorityColorInt(p int) int {
	switch p {
	case diff.High:
		return 0xd73a49
	case diff.Medium:
		return 0xd29922
	default:
		return 0x57606a
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

// Email delivers alerts over SMTP.
type Email struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string
	send     func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

// NewEmail builds an SMTP email notifier. Port defaults to 587 (submission).
func NewEmail(host string, port int, username, password, from string, to []string) *Email {
	if port == 0 {
		port = 587
	}
	return &Email{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		From:     from,
		To:       to,
		send:     smtp.SendMail,
	}
}

// Notify emails the changes. It is a no-op when there are no changes.
func (e *Email) Notify(ctx context.Context, scope string, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	subject := fmt.Sprintf("reconsentry: %d change(s) on %s", len(changes), scope)
	msg := buildEmail(e.From, e.To, subject, RenderText(scope, changes))

	var auth smtp.Auth
	if e.Username != "" {
		auth = smtp.PlainAuth("", e.Username, e.Password, e.Host)
	}
	send := e.send
	if send == nil {
		send = smtp.SendMail
	}
	if err := send(fmt.Sprintf("%s:%d", e.Host, e.Port), auth, e.From, e.To, msg); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return nil
}

// buildEmail assembles a minimal RFC 5322 plain-text message.
func buildEmail(from string, to []string, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	b.WriteString("\r\n")
	return []byte(b.String())
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
