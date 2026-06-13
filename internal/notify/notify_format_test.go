package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/maruftak/reconsentry/internal/diff"
)

func TestGroupByPriorityOrdersAndBuckets(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.NewTech, Host: "low.example.com", Priority: diff.Low},
		{Kind: diff.NewHost, Host: "high.example.com", Priority: diff.High},
		{Kind: diff.StatusChange, Host: "med.example.com", Priority: diff.Medium},
		{Kind: diff.HostGone, Host: "low2.example.com", Priority: 99}, // odd value → low
	}
	groups := groupByPriority(changes)
	if len(groups) != 3 {
		t.Fatalf("want 3 groups, got %d", len(groups))
	}
	if groups[0].priority != diff.High || groups[1].priority != diff.Medium || groups[2].priority != diff.Low {
		t.Fatalf("groups out of order: %d,%d,%d", groups[0].priority, groups[1].priority, groups[2].priority)
	}
	if len(groups[2].changes) != 2 {
		t.Errorf("odd priority should fall into low bucket, got %d low changes", len(groups[2].changes))
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("short string unchanged, got %q", got)
	}
	got := truncate("hello world", 8)
	if len([]byte(got)) > 8 {
		t.Errorf("truncate exceeded limit: %q (%d bytes)", got, len(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated string should end with ellipsis, got %q", got)
	}
	// Multi-byte: must stay within the limit and never split a rune.
	multi := truncate(strings.Repeat("日", 10), 8)
	if len([]byte(multi)) > 8 {
		t.Errorf("multibyte truncate exceeded limit: %d bytes", len(multi))
	}
	if !utf8.ValidString(multi) {
		t.Errorf("truncate split a rune: %q", multi)
	}
}

func TestChunkLinesRespectsLimit(t *testing.T) {
	lines := []string{"aaaa", "bbbb", "cccc", "dddd"} // 4 bytes each
	chunks := chunkLines(lines, 10)                   // fits ~2 lines (4+1+4=9) per chunk
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks under a tight limit, got %d: %v", len(chunks), chunks)
	}
	for _, c := range chunks {
		if len(c) > 10 {
			t.Errorf("chunk exceeds limit: %q (%d bytes)", c, len(c))
		}
	}
}

func TestSlackBlocksStayUnderSectionLimit(t *testing.T) {
	// One high-priority change with a huge detail forces section chunking.
	changes := []diff.Change{
		{Kind: diff.NewHost, Host: "a.example.com", Detail: strings.Repeat("x", 5000), Priority: diff.High},
	}
	blocks := slackChangeBlocks(changes)
	for _, b := range blocks {
		text := b["text"].(map[string]string)["text"]
		if len([]byte(text)) > slackSectionLimit {
			t.Errorf("slack section exceeds %d chars: %d", slackSectionLimit, len(text))
		}
	}
}

func TestSlackBlocksGroupedWithEmoji(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.NewHost, Host: "h.example.com", Detail: "x", Priority: diff.High},
		{Kind: diff.NewTech, Host: "l.example.com", Detail: "y", Priority: diff.Low},
	}
	encoded, _ := json.Marshal(slackChangeBlocks(changes))
	s := string(encoded)
	for _, want := range []string{"🔴", "⚪", "high priority", "low priority"} {
		if !strings.Contains(s, want) {
			t.Errorf("slack blocks missing %q: %s", want, s)
		}
	}
}

func TestDiscordFieldsCappedAt25(t *testing.T) {
	// 30 distinct low-priority changes, each forced into its own field by a
	// near-limit detail, must not exceed Discord's 25-field cap.
	var changes []diff.Change
	for i := 0; i < 30; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.NewTech,
			Host:     fmt.Sprintf("h%d.example.com", i),
			Detail:   strings.Repeat("z", discordFieldLimit),
			Priority: diff.Low,
		})
	}
	fields := discordFields(changes)
	if len(fields) > discordMaxFields {
		t.Fatalf("discord fields exceed cap: %d > %d", len(fields), discordMaxFields)
	}
	last := fields[len(fields)-1]
	if !strings.Contains(last["value"].(string), "more field(s) omitted") {
		t.Errorf("overflow should leave a truncation note, got %v", last)
	}
}

func TestDiscordFieldValuesUnderLimit(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.NewHost, Host: "a.example.com", Detail: strings.Repeat("y", 4000), Priority: diff.High},
	}
	for _, f := range discordFields(changes) {
		if v := f["value"].(string); len([]byte(v)) > discordFieldLimit {
			t.Errorf("discord field value exceeds %d: %d", discordFieldLimit, len(v))
		}
	}
}
