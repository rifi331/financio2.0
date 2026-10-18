package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/store"
)

type fakeStore struct {
	txns []store.DashboardTxn
	cats []string
	gotF store.DashboardFilter
}

func (f *fakeStore) ListDashboardTransactions(ctx context.Context, fl store.DashboardFilter, limit int) ([]store.DashboardTxn, error) {
	f.gotF = fl
	return f.txns, nil
}

func (f *fakeStore) ListActiveCategories(ctx context.Context) ([]string, error) {
	return f.cats, nil
}

func newTestServer() (*Server, *fakeStore, *Ring) {
	fs := &fakeStore{
		cats: []string{"Meals", "Housing"},
		txns: []store.DashboardTxn{{
			Transaction: store.Transaction{
				ID: 1, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR",
				TotalAmount: "10.50", Direction: "expense", EntryDate: time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC),
				Source: "text",
			},
			UserName: "Rifi Anggriawan",
		}},
	}
	ring := NewRing(10)
	ring.Append(`{"level":"INFO","msg":"hello"}`)
	return New(fs, ring, "sekret", time.UTC), fs, ring
}

func TestDisabledWithoutToken(t *testing.T) {
	s, _, _ := newTestServer()
	s.token = ""
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestHealthzAlwaysOpen(t *testing.T) {
	s, _, _ := newTestServer()
	s.token = ""
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("healthz broken: %d %q", rec.Code, rec.Body.String())
	}
}

func TestRedirectToLoginWhenAnonymous(t *testing.T) {
	s, _, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want 303 to login, got %d", rec.Code)
	}
}

func TestKeyParamSetsCookieAndShowsData(t *testing.T) {
	s, fs, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/?key=sekret", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"nasi lemak", "Meals", "MYR 10.50", "Rifi Anggriawan"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), cookieName) {
		t.Error("expected auth cookie to be set")
	}
	if fs.gotF.Category != "" || fs.gotF.Direction != "" {
		t.Errorf("unexpected filters: %+v", fs.gotF)
	}
}

func TestFiltersPassedToStore(t *testing.T) {
	s, fs, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/?key=sekret&month=2026-03&category=Meals&direction=income", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if fs.gotF.Category != "Meals" || fs.gotF.Direction != "income" {
		t.Errorf("filters not applied: %+v", fs.gotF)
	}
	if got := fs.gotF.From.Format("2006-01"); got != "2026-03" {
		t.Errorf("from month = %s", got)
	}
	if !fs.gotF.To.Equal(fs.gotF.From.AddDate(0, 1, 0)) {
		t.Errorf("to should be exclusive next month: %v", fs.gotF.To)
	}
}

func TestInvalidMonthRejected(t *testing.T) {
	s, _, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/?key=sekret&month=notamonth", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestWrongTokenRejected(t *testing.T) {
	s, _, _ := newTestServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=nope"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Wrong token") {
		t.Fatalf("wrong token should re-render login: %d", rec.Code)
	}
}

func TestLoginFlowSetsCookieThenGrantsAccess(t *testing.T) {
	s, _, _ := newTestServer()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/login", strings.NewReader("token=sekret"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("want redirect, got %d", rec.Code)
	}
	cookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(cookie, cookieName) {
		t.Fatal("no cookie set")
	}

	getReq := httptest.NewRequest("GET", "/", nil)
	getReq.Header.Set("Cookie", cookie)
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, getReq)
	if rec2.Code != http.StatusOK {
		t.Fatalf("cookie auth failed: %d", rec2.Code)
	}
}

func TestLogsPages(t *testing.T) {
	s, _, _ := newTestServer()
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest("GET", "/logs?key=sekret", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("logs page broken: %d", rec.Code)
	}
	rec2 := httptest.NewRecorder()
	s.ServeHTTP(rec2, httptest.NewRequest("GET", "/api/logs?key=sekret", nil))
	if ct := rec2.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("logs json content-type = %s", ct)
	}
	if !strings.Contains(rec2.Body.String(), "hello") {
		t.Error("logs json missing line")
	}
}

func TestRingTail(t *testing.T) {
	r := NewRing(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		r.Append(s)
	}
	if got := strings.Join(r.Tail(0), ""); got != "cde" {
		t.Errorf("ring should keep last 3, got %q", got)
	}
	if n, _ := r.Write([]byte("x\n")); n != 2 {
		t.Error("Write should report full length")
	}
}
