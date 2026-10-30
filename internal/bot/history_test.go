package bot

import (
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/ai"
)

func TestHistoryRingAndIsolation(t *testing.T) {
	h := NewHistory(3)
	for i := 0; i < 5; i++ {
		h.Add(1, "u", "a")
	}
	if got := h.Messages(1); len(got) != 6 { // 3 turns = 6 messages
		t.Errorf("ring should keep 3 turns, got %d messages", len(got))
	}
	h.Add(2, "other chat", "other reply")
	if got := h.Messages(2); len(got) != 2 {
		t.Errorf("chat 2 should have its own history, got %d", len(got))
	}
	if strings.Contains(strings.Join(msgTexts(h.Messages(2)), " "), "other chat") == false {
		t.Error("chat 2 history lost its own content")
	}
	for _, m := range h.Messages(1) {
		if strings.Contains(contentString(m), "other chat") {
			t.Error("chat isolation violated")
		}
	}
}

func TestHistoryTruncatesAssistant(t *testing.T) {
	h := NewHistory(2)
	long := strings.Repeat("x", 500)
	h.Add(1, "q", long)
	for _, m := range h.Messages(1) {
		if len(contentString(m)) > 220 {
			t.Error("assistant turn not truncated to ~200 chars")
		}
	}
}

func msgTexts(ms []ai.Message) []string {
	out := make([]string, len(ms))
	for i, m := range ms {
		out[i] = contentString(m)
	}
	return out
}

func contentString(m ai.Message) string {
	if s, ok := m.Content.(string); ok {
		return s
	}
	return ""
}
