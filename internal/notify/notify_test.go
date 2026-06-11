package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/maruftak/newfound/internal/diff"
)

func sampleChanges() []diff.Change {
	return []diff.Change{
		{Kind: diff.NewHost, Host: "a.example.com", Detail: "new live host [200 nginx]", Priority: diff.High},
	}
}

func TestRenderText(t *testing.T) {
	out := RenderText("prog", sampleChanges())
	for _, want := range []string{"prog", "NEW_HOST", "a.example.com"} {
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
