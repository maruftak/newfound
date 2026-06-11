package notify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/smtp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/maruftak/reconsentry/internal/diff"
)

func sampleChanges() []diff.Change {
	return []diff.Change{
		{Kind: diff.NewHost, Host: "a.example.com", Detail: "new live host [200 nginx]", Priority: diff.High},
	}
}

func TestRenderText(t *testing.T) {
	out := RenderText("prog", sampleChanges())
	for _, want := range []string{"prog", "NEW_HOST", "a.example.com", "https://a.example.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q: %q", want, out)
		}
	}
}

func TestWebhookEmptyIsNoop(t *testing.T) {
	w := NewWebhook("http://127.0.0.1:0", "generic")
	if err := w.Notify(context.Background(), "prog", nil); err != nil {
		t.Errorf("empty changes should be a no-op, got %v", err)
	}
}

func TestWebhookSlackPayload(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL, "slack")
	if err := w.Notify(context.Background(), "prog", sampleChanges()); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["text"]; !ok {
		t.Errorf("slack payload should have a text field, got %v", body)
	}
}

func TestWebhookRetriesTransientThenSucceeds(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			rw.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL, "generic")
	if err := w.Notify(context.Background(), "prog", sampleChanges()); err != nil {
		t.Fatalf("should succeed after transient failures, got %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Errorf("want 3 attempts, got %d", got)
	}
}

func TestTelegramSendsMessage(t *testing.T) {
	var gotPath string
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tg := NewTelegram("123:ABC", "987")
	tg.BaseURL = srv.URL
	if err := tg.Notify(context.Background(), "prog", sampleChanges()); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/bot123:ABC/sendMessage" {
		t.Errorf("unexpected request path: %q", gotPath)
	}
	if body["chat_id"] != "987" {
		t.Errorf("chat_id should be sent, got %q", body["chat_id"])
	}
	if !strings.Contains(body["text"], "NEW_HOST") {
		t.Errorf("text should carry the change, got %q", body["text"])
	}
}

func TestTelegramEmptyIsNoop(t *testing.T) {
	tg := NewTelegram("t", "c")
	tg.BaseURL = "http://127.0.0.1:0"
	if err := tg.Notify(context.Background(), "prog", nil); err != nil {
		t.Errorf("empty changes should be a no-op, got %v", err)
	}
}

func TestEmailSends(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	e := &Email{
		Host: "smtp.example.com", Port: 587, Username: "u", Password: "p",
		From: "from@example.com", To: []string{"a@example.com", "b@example.com"},
		send: func(addr string, _ smtp.Auth, from string, to []string, msg []byte) error {
			gotAddr, gotFrom, gotTo, gotMsg = addr, from, to, msg
			return nil
		},
	}
	if err := e.Notify(context.Background(), "prog", sampleChanges()); err != nil {
		t.Fatal(err)
	}
	if gotAddr != "smtp.example.com:587" {
		t.Errorf("addr: %q", gotAddr)
	}
	if gotFrom != "from@example.com" || len(gotTo) != 2 {
		t.Errorf("from/to: %q %v", gotFrom, gotTo)
	}
	s := string(gotMsg)
	for _, want := range []string{"Subject: reconsentry: 1 change(s) on prog", "To: a@example.com, b@example.com", "NEW_HOST"} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q in:\n%s", want, s)
		}
	}
}

func TestEmailEmptyIsNoop(t *testing.T) {
	e := &Email{send: func(string, smtp.Auth, string, []string, []byte) error {
		return errors.New("should not send for empty changes")
	}}
	if err := e.Notify(context.Background(), "prog", nil); err != nil {
		t.Errorf("empty changes should be a no-op, got %v", err)
	}
}

func TestWebhookDoesNotRetryClientError(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&attempts, 1)
		rw.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	w := NewWebhook(srv.URL, "generic")
	if err := w.Notify(context.Background(), "prog", sampleChanges()); err == nil {
		t.Fatal("4xx should return an error")
	}
	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Errorf("4xx must not be retried; want 1 attempt, got %d", got)
	}
}
