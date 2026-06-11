package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
