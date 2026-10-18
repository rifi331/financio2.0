package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

func recapServer(t *testing.T, raw string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ResponseFormat == nil || !req.ResponseFormat.JSONSchema.Strict {
			t.Errorf("response_format wrong: %+v", req.ResponseFormat)
		}
		if !strings.Contains(req.Messages[1].Content, "[Today's date: 2026-10-18]") {
			t.Errorf("today not injected: %q", req.Messages[1].Content)
		}
		if !strings.Contains(req.Messages[0].Content, "this list: Meals") {
			t.Errorf("categories not in system prompt: %q", req.Messages[0].Content)
		}
		_, _ = w.Write([]byte(okBody(raw)))
	}))
}

func TestParseRecapFilterMonth(t *testing.T) {
	srv := recapServer(t, `{"is_recap_request":true,"date_from":"2025-11-01","date_to":"2025-11-30","categories":[],"language":"EN"}`)
	defer srv.Close()

	f, err := newTestClient(srv.URL).ParseRecapFilter(context.Background(),
		"recap nov 2025", today, []string{"Meals", "Groceries"})
	if err != nil {
		t.Fatalf("ParseRecapFilter: %v", err)
	}
	if !f.IsRecapRequest || f.DateFrom != "2025-11-01" || f.DateTo != "2025-11-30" || f.Language != "EN" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseRecapFilterThisMonthCategories(t *testing.T) {
	srv := recapServer(t, `{"is_recap_request":true,"date_from":"2026-10-01","date_to":"2026-10-18","categories":["Meals"],"language":"ID"}`)
	defer srv.Close()

	f, err := newTestClient(srv.URL).ParseRecapFilter(context.Background(),
		"rekap bulan ini makan", today, []string{"Meals", "Groceries"})
	if err != nil {
		t.Fatalf("ParseRecapFilter: %v", err)
	}
	if !f.IsRecapRequest || f.Language != "ID" || len(f.Categories) != 1 || f.Categories[0] != "Meals" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseRecapFilterToday(t *testing.T) {
	srv := recapServer(t, `{"is_recap_request":true,"date_from":"2026-10-18","date_to":"2026-10-18","categories":[],"language":"EN"}`)
	defer srv.Close()

	f, err := newTestClient(srv.URL).ParseRecapFilter(context.Background(),
		"recap today", today, []string{"Meals"})
	if err != nil {
		t.Fatalf("ParseRecapFilter: %v", err)
	}
	if f.DateFrom != "2026-10-18" || f.DateTo != "2026-10-18" {
		t.Errorf("filter = %+v", f)
	}
}

func TestParseRecapFilterGarbageNotRecap(t *testing.T) {
	srv := recapServer(t, `{"is_recap_request":false,"date_from":"","date_to":"","categories":[],"language":"EN"}`)
	defer srv.Close()

	f, err := newTestClient(srv.URL).ParseRecapFilter(context.Background(),
		"hello garbage text", today, []string{"Meals"})
	if err != nil {
		t.Fatalf("ParseRecapFilter: %v", err)
	}
	if f.IsRecapRequest {
		t.Errorf("filter = %+v, want is_recap_request=false", f)
	}
}

func TestParseRecapFilterMalformed(t *testing.T) {
	srv := recapServer(t, `{not json`)
	defer srv.Close()

	if _, err := newTestClient(srv.URL).ParseRecapFilter(context.Background(),
		"recap nov 2025", today, []string{"Meals"}); err == nil {
		t.Fatal("expected decode error")
	}
}
