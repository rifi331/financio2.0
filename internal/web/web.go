// Package web serves the read-only dashboard: recent transactions with
// filters, per-currency totals, and the in-memory log tail.
package web

import (
	"strings"
	"sync"
)

// Ring is the shared in-memory log tail.
type Ring struct {
	mu    sync.Mutex
	lines []string
	max   int
}

// NewRing creates a ring holding at most max log lines.
func NewRing(max int) *Ring { return &Ring{max: max} }

// Append adds one formatted log line.
func (r *Ring) Append(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, s)
	if len(r.lines) > r.max {
		r.lines = r.lines[len(r.lines)-r.max:]
	}
}

// Tail returns the last n lines (or all if n <= 0).
func (r *Ring) Tail(n int) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.lines) {
		n = len(r.lines)
	}
	out := make([]string, n)
	copy(out, r.lines[len(r.lines)-n:])
	return out
}

// Write implements io.Writer so the ring can be MultiWriter-ed with stdout.
// slog's JSON handler emits one complete line per Write.
func (r *Ring) Write(p []byte) (int, error) {
	if s := strings.TrimRight(string(p), "\n"); s != "" {
		r.Append(s)
	}
	return len(p), nil
}
