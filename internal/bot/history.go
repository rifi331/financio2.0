package bot

import (
	"sync"

	"github.com/rifi331/financio/internal/ai"
)

// History is an in-memory per-chat ring of recent conversation turns,
// replayed to the model as context on text entries. Restart clears it —
// acceptable for expense entry (models are stateless; this is the memory).
type History struct {
	mu    sync.Mutex
	turns map[int64][]ai.Message
	max   int // turns per chat
}

// NewHistory keeps the last n turns per chat.
func NewHistory(n int) *History {
	if n <= 0 {
		n = 10
	}
	return &History{turns: map[int64][]ai.Message{}, max: n}
}

// Add records one user/assistant turn for a chat. The assistant side is
// truncated — confirmations repeat themselves and blow up the prompt.
func (h *History) Add(chatID int64, user, assistant string) {
	if h == nil {
		return
	}
	if len(assistant) > 200 {
		assistant = assistant[:200]
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.turns[chatID] = append(h.turns[chatID],
		ai.Message{Role: "user", Content: user},
		ai.Message{Role: "assistant", Content: assistant},
	)
	if extra := len(h.turns[chatID]) - h.max*2; extra > 0 {
		h.turns[chatID] = h.turns[chatID][extra:]
	}
}

// Messages returns a copy of the chat's history turns in order.
func (h *History) Messages(chatID int64) []ai.Message {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]ai.Message, len(h.turns[chatID]))
	copy(out, h.turns[chatID])
	return out
}
