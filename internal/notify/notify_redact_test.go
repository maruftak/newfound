package notify

import (
	"context"
	"strings"
	"testing"

	"github.com/maruftak/reconsentry/internal/diff"
)

// A failed delivery must not leak the bot token (which lives in the request URL)
// into the returned error / logs.
func TestTelegramTransportErrorHidesToken(t *testing.T) {
	tg := NewTelegram("SUPERSECRET:AAtoken", "123")
	tg.BaseURL = "http://127.0.0.1:1" // nothing listening -> transport error carrying the URL

	changes := []diff.Change{{Kind: diff.NewHost, Host: "a.example.com", Detail: "new", Priority: diff.High}}
	err := tg.Notify(context.Background(), "prog", changes)
	if err == nil {
		t.Fatal("expected a delivery error")
	}
	if strings.Contains(err.Error(), "SUPERSECRET") {
		t.Fatalf("bot token leaked into error: %v", err)
	}
}
