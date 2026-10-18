package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func okBody(content string) string {
	inner, _ := json.Marshal(content)
	return `{"choices":[{"message":{"role":"assistant","content":` + string(inner) + `}}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`
}

func newTestClient(srvURL string) *Client {
	c := NewClient(srvURL, "test-key", "gpt-4o-mini")
	c.HTTP = &http.Client{}
	return c
}

func TestChatSuccess(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_, _ = w.Write([]byte(okBody("hello world")))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	out, err := c.Chat(context.Background(), ChatRequest{Messages: []Message{{Role: "user", Content: "hi"}}})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out != "hello world" {
		t.Errorf("out = %q", out)
	}
	if gotPath != "/chat/completions" || gotAuth != "Bearer test-key" {
		t.Errorf("path=%q auth=%q", gotPath, gotAuth)
	}
	if !strings.Contains(gotBody, `"role":"user"`) {
		t.Errorf("body = %q", gotBody)
	}
}

func TestChatRetriesOn429(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(okBody("after retry")))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL)
	out, err := c.Chat(context.Background(), ChatRequest{})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if out != "after retry" {
		t.Errorf("out = %q", out)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestChatRetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(okBody("ok")))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).Chat(context.Background(), ChatRequest{}); err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

func TestChatNoRetryOn400(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error on 400")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 (no retries)", calls)
	}
}

func TestChatMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error on malformed JSON")
	}
}

func TestChatGivesUpAfter3Attempts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).Chat(context.Background(), ChatRequest{}); err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestParseEntrySuccess(t *testing.T) {
	entry := ParsedEntry{IsTransaction: true, Item: "nasi lemak", Category: "Meals",
		Direction: "expense", Amount: 10, Currency: "MYR", Date: "2026-10-18"}
	raw, _ := json.Marshal(entry)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" ||
			!req.ResponseFormat.JSONSchema.Strict {
			t.Errorf("response_format wrong: %+v", req.ResponseFormat)
		}
		if !strings.Contains(req.Messages[1].Content, "[Current date: 2026-10-18]") {
			t.Errorf("current date not injected: %q", req.Messages[1].Content)
		}
		if !strings.Contains(req.Messages[0].Content, "Meals") ||
			!strings.Contains(req.Messages[0].Content, "MYR, IDR") {
			t.Errorf("categories/currencies not in system prompt: %q", req.Messages[0].Content)
		}
		_, _ = w.Write([]byte(okBody(string(raw))))
	}))
	defer srv.Close()

	p, err := newTestClient(srv.URL).ParseEntry(context.Background(), EntryInput{
		Message: "10rm nasi lemak", CurrentDate: "2026-10-18",
		Categories: []string{"Meals", "Groceries"}, Currencies: []string{"MYR", "IDR"},
		DefaultCurrency: "MYR",
	})
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if !p.IsTransaction || p.Item != "nasi lemak" || p.Amount != 10 || p.Currency != "MYR" {
		t.Errorf("parsed = %+v", p)
	}
}

func TestParseEntrySchemaMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wrong shape: is_transaction is a string, details is an object.
		_, _ = w.Write([]byte(okBody(`{"is_transaction":"yes","details":{"a":1}}`)))
	}))
	defer srv.Close()

	if _, err := newTestClient(srv.URL).ParseEntry(context.Background(), EntryInput{
		Categories: []string{"Meals"}, Currencies: []string{"MYR"}, DefaultCurrency: "MYR",
	}); err == nil {
		t.Fatal("expected schema mismatch error")
	}
}

func TestParseEntryEmptyDirectionDefaultsToExpense(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(okBody(`{"is_transaction":true,"item":"x","category":"Meals","amount":1,"currency":"MYR","date":"2026-10-18","details":[]}`)))
	}))
	defer srv.Close()

	p, err := newTestClient(srv.URL).ParseEntry(context.Background(), EntryInput{
		Categories: []string{"Meals"}, Currencies: []string{"MYR"}, DefaultCurrency: "MYR",
	})
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if p.Direction != "expense" {
		t.Errorf("direction = %q, want expense default", p.Direction)
	}
}

func TestChatPerAttemptTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte(okBody("late")))
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := newTestClient(srv.URL).Chat(ctx, ChatRequest{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if time.Since(start) > time.Second {
		t.Error("attempt did not respect the 30s per-attempt ctx deadline promptly")
	}
}
