package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseEntrySendsHistory(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"content":"{}"}}]}`)
	}))
	defer srv.Close()

	_, _ = newTestClient(srv.URL).ParseEntry(context.Background(), EntryInput{
		Message: "another one",
		History: []Message{
			{Role: "user", Content: "10rm nasi lemak yesterday"},
			{Role: "assistant", Content: "saved"},
		},
	})
	if strings.Count(body, "\"role\"") != 4 { // system + 2 history + current
		t.Errorf("history not sent: %d roles", strings.Count(body, "\"role\""))
	}
	if !strings.Contains(body, "nasi lemak yesterday") {
		t.Error("history content missing from request")
	}
}
