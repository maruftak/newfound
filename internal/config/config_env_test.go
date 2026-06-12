package config

import "testing"

func TestParseExpandsEnvSecrets(t *testing.T) {
	t.Setenv("RS_TEST_TOKEN", "secret-123")
	t.Setenv("RS_TEST_CHAT", "42")
	y := []byte(`name: x
targets: [a.com]
notify:
  telegram:
    - token: ${RS_TEST_TOKEN}
      chat_id: "${RS_TEST_CHAT}"
`)
	c, err := Parse(y)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Notify.Telegram) != 1 {
		t.Fatalf("want 1 telegram target, got %d", len(c.Notify.Telegram))
	}
	if got := c.Notify.Telegram[0]; got.Token != "secret-123" || got.ChatID != "42" {
		t.Fatalf("env not expanded: %+v", got)
	}
}

func TestParseUnsetEnvFailsValidation(t *testing.T) {
	y := []byte(`name: x
targets: [a.com]
notify:
  telegram:
    - token: ${RS_DEFINITELY_UNSET_VAR}
      chat_id: "1"
`)
	if _, err := Parse(y); err == nil {
		t.Error("unset ${VAR} should expand to empty and fail validation")
	}
}
