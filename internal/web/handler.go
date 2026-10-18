package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/recap"
	"github.com/rifi331/financio/internal/store"
)

const cookieName = "financio_dash"

// TxnLister is the store surface the dashboard needs (satisfied by *store.Store).
type TxnLister interface {
	ListDashboardTransactions(ctx context.Context, f store.DashboardFilter, limit int) ([]store.DashboardTxn, error)
	ListActiveCategories(ctx context.Context) ([]string, error)
}

// Server serves the dashboard. A nil/empty token disables it (503).
type Server struct {
	store TxnLister
	ring  *Ring
	token string // raw DASHBOARD_TOKEN
	tz    *time.Location
	mux   *http.ServeMux
}

// compile-time: *store.Store satisfies TxnLister.
var _ TxnLister = (*store.Store)(nil)

// New builds the dashboard server. tz may be nil (UTC assumed).
func New(st TxnLister, ring *Ring, token string, tz *time.Location) *Server {
	if tz == nil {
		tz = time.UTC
	}
	s := &Server{store: st, ring: ring, token: token, tz: tz, mux: http.NewServeMux()}
	s.mux.HandleFunc("/", s.handleDashboard)
	s.mux.HandleFunc("/logs", s.handleLogs)
	s.mux.HandleFunc("/api/logs", s.handleLogsJSON)
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/logout", s.handleLogout)
	s.mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/healthz" && s.token == "" {
		http.Error(w, "dashboard disabled: set DASHBOARD_TOKEN to enable", http.StatusServiceUnavailable)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func tokenHash(token string) string {
	h := sha256.Sum256([]byte("financio:" + token))
	return hex.EncodeToString(h[:])
}

func (s *Server) authorized(r *http.Request) bool {
	if c, err := r.Cookie(cookieName); err == nil && c.Value != "" {
		return subtle.ConstantTimeCompare([]byte(c.Value), []byte(tokenHash(s.token))) == 1
	}
	// Allow ?key=... for simple bookmarking.
	if k := r.URL.Query().Get("key"); k != "" &&
		subtle.ConstantTimeCompare([]byte(k), []byte(s.token)) == 1 {
		return true
	}
	return false
}

func (s *Server) setAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    tokenHash(s.token),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authorized(r) {
		if _, err := r.Cookie(cookieName); err != nil {
			s.setAuthCookie(w) // came in via ?key=
		}
		return true
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
	return false
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.PostFormValue("token")), []byte(s.token)) == 1 {
			s.setAuthCookie(w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		renderPage(w, loginTmpl, loginData{Wrong: true})
		return
	}
	renderPage(w, loginTmpl, loginData{})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) monthBounds(month string) (from, to time.Time, ok bool) {
	if month == "" {
		now := time.Now().In(s.tz)
		month = now.Format("2006-01")
	}
	t, err := time.ParseInLocation("2006-01", month, s.tz)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	return t, t.AddDate(0, 1, 0), true
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if !s.requireAuth(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	q := r.URL.Query()
	month := q.Get("month")
	category := q.Get("category")
	direction := q.Get("direction")
	from, to, ok := s.monthBounds(month)
	if !ok {
		http.Error(w, "month must look like 2026-10", http.StatusBadRequest)
		return
	}
	if direction != "expense" && direction != "income" {
		direction = ""
	}

	f := store.DashboardFilter{From: from, To: to, Category: category, Direction: direction}
	txns, err := s.store.ListDashboardTransactions(ctx, f, 500)
	if err != nil {
		http.Error(w, "database error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	cats, err := s.store.ListActiveCategories(ctx)
	if err != nil {
		http.Error(w, "database error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Net per-currency totals (income minus expenses) for the filtered set.
	net := map[string]float64{}
	rows := make([]dashRow, 0, len(txns))
	for _, t := range txns {
		amt, _ := strconv.ParseFloat(t.TotalAmount, 64)
		if t.Direction == "expense" {
			amt = -amt
		}
		net[t.CurrencyCode] += amt
		rows = append(rows, dashRow{
			Date:     t.EntryDate.In(s.tz).Format("02 Jan"),
			UserName: t.UserName,
			Item:     t.Item,
			Category: t.CategoryName,
			Amount:   recap.FormatAmountCode(t.CurrencyCode, dpOf(t.CurrencyCode), t.TotalAmount),
			Income:   t.Direction == "income",
			Source:   t.Source,
		})
	}
	totalLines := make([]string, 0, len(net))
	for _, code := range sortedKeys(net) {
		totalLines = append(totalLines, recap.FormatAmountCode(code, dpOf(code), formatFloat(net[code])))
	}

	data := dashData{
		Month:      from.In(s.tz).Format("2006-01"),
		Categories: cats,
		Category:   category,
		Direction:  direction,
		Rows:       rows,
		Totals:     strings.Join(totalLines, " + "),
		Count:      len(rows),
	}
	renderPage(w, dashTmpl, data)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	renderPage(w, logsTmpl, logsData{Lines: s.ring.Tail(0)})
}

func (s *Server) handleLogsJSON(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": s.ring.Tail(0)})
}

func dpOf(code string) int {
	if code == "IDR" {
		return 0
	}
	return 2
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}
