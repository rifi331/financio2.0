package bot

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/auth"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

// --- fakes ---

type fakeStore struct {
	inserted     []store.NewTransaction
	rows         []store.Transaction
	nextID       int64
	unauthorized bool // when true, no user is known and EnsureUser records the attempt
	ensured      []int64
	lastFilter   *store.TxnFilter
	dupGroups    []store.DuplicateGroup
	prompts      map[string]string
	sysparams    map[string]string
	budgets      map[[2]string]string
	lastDupRange *[2]time.Time
}

func (f *fakeStore) ListActiveCategories(context.Context) ([]string, error) {
	return []string{"Groceries", "Meals", "Housing", "Others"}, nil
}

func (f *fakeStore) GetPrompt(_ context.Context, name string) (string, bool, error) {
	t, ok := f.prompts[name]
	return t, ok, nil
}

func (f *fakeStore) SetPrompt(_ context.Context, name, template string) error {
	if f.prompts == nil {
		f.prompts = map[string]string{}
	}
	f.prompts[name] = template
	return nil
}

func (f *fakeStore) DeletePrompt(_ context.Context, name string) (bool, error) {
	if _, ok := f.prompts[name]; !ok {
		return false, nil
	}
	delete(f.prompts, name)
	return true, nil
}

func (f *fakeStore) SetBudget(_ context.Context, cat, code, limit string) error {
	if f.budgets == nil {
		f.budgets = map[[2]string]string{}
	}
	f.budgets[[2]string{cat, code}] = limit
	return nil
}

func (f *fakeStore) DeleteBudget(_ context.Context, cat, code string) (bool, error) {
	k := [2]string{cat, code}
	if _, ok := f.budgets[k]; !ok {
		return false, nil
	}
	delete(f.budgets, k)
	return true, nil
}

func (f *fakeStore) ListBudgets(_ context.Context) ([]store.Budget, error) {
	var out []store.Budget
	for k, v := range f.budgets {
		out = append(out, store.Budget{CategoryName: k[0], CurrencyCode: k[1], MonthlyLimit: v})
	}
	return out, nil
}

func (f *fakeStore) MonthCategorySpend(_ context.Context, cat, code string, t time.Time) (string, error) {
	total := 0.0
	for _, r := range f.rows {
		if r.CategoryName == cat && r.CurrencyCode == code && r.Direction == "expense" {
			amt, _ := strconv.ParseFloat(r.TotalAmount, 64)
			total += amt
		}
	}
	return strconv.FormatFloat(total, 'f', 2, 64), nil
}

func (f *fakeStore) GetSysParam(_ context.Context, key string) (string, bool, error) {
	v, ok := f.sysparams[key]
	return v, ok, nil
}

func (f *fakeStore) SetSysParam(_ context.Context, key, value string) error {
	if f.sysparams == nil {
		f.sysparams = map[string]string{}
	}
	f.sysparams[key] = value
	return nil
}

func (f *fakeStore) ListSysParams(_ context.Context) ([]store.SysParam, error) {
	var out []store.SysParam
	for k, v := range f.sysparams {
		out = append(out, store.SysParam{Key: k, Value: v})
	}
	return out, nil
}

func (f *fakeStore) ListPrompts(_ context.Context) ([]store.Prompt, error) {
	var out []store.Prompt
	for n, t := range f.prompts {
		out = append(out, store.Prompt{Name: n, Template: t})
	}
	return out, nil
}

func (f *fakeStore) ListDuplicateGroups(ctx context.Context, userID int64, from, to time.Time) ([]store.DuplicateGroup, error) {
	r := [2]time.Time{from, to}
	f.lastDupRange = &r
	return f.dupGroups, nil
}

func (f *fakeStore) ListActiveCurrencies(context.Context) ([]store.Currency, error) {
	return []store.Currency{
		{Code: "MYR", Symbol: "RM", DecimalPlaces: 2, SortOrder: 1},
		{Code: "IDR", Symbol: "Rp", DecimalPlaces: 0, SortOrder: 2},
		{Code: "USD", Symbol: "$", DecimalPlaces: 2, SortOrder: 3},
	}, nil
}

func (f *fakeStore) InsertTransaction(_ context.Context, txn store.NewTransaction) (store.Transaction, error) {
	f.nextID++
	t := store.Transaction{
		ID: f.nextID, UserID: txn.UserID, Item: txn.Item, CategoryName: txn.Category,
		CurrencyCode: txn.CurrencyCode, TotalAmount: txn.TotalAmount,
		Direction: txn.Direction, EntryDate: txn.EntryDate, Source: txn.Source,
		RawMessage: txn.RawMessage, CreatedAt: time.Now(),
	}
	f.inserted = append(f.inserted, txn)
	f.rows = append(f.rows, t)
	return t, nil
}

func (f *fakeStore) DeleteTransactionByID(_ context.Context, id, userID int64) (store.Transaction, bool, error) {
	for i, t := range f.rows {
		if t.ID == id {
			if t.UserID != userID {
				return store.Transaction{}, false, nil // not owned -> same as not found
			}
			f.rows = append(f.rows[:i], f.rows[i+1:]...)
			return t, true, nil
		}
	}
	return store.Transaction{}, false, nil
}

func (f *fakeStore) ListTransactions(_ context.Context, flt store.TxnFilter) ([]store.Transaction, error) {
	f.lastFilter = &flt
	var out []store.Transaction
	for _, t := range f.rows {
		if t.UserID != flt.UserID {
			continue
		}
		if !flt.From.IsZero() && t.EntryDate.Before(flt.From) {
			continue
		}
		if !flt.To.IsZero() && t.EntryDate.After(flt.To) {
			continue
		}
		if len(flt.Categories) > 0 && !categoryIn(flt.Categories, t.CategoryName) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func categoryIn(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (f *fakeStore) GetUserByTelegramID(_ context.Context, id int64) (store.User, bool, error) {
	if f.unauthorized {
		return store.User{}, false, nil
	}
	return store.User{ID: id, DisplayName: "Rifi Anggriawan", IsActive: true}, true, nil
}

func (f *fakeStore) EnsureUser(_ context.Context, id int64, name string) (store.User, error) {
	f.ensured = append(f.ensured, id)
	return store.User{ID: id, DisplayName: name, IsActive: true}, nil
}

type fakeAI struct {
	// responses maps raw user text to the parsed entry the "model" returns.
	responses map[string]ai.ParsedEntry
	requests  []ai.EntryInput

	recapResponses map[string]ai.RecapFilter
	recapRequests  []string

	photoResponse []ai.ParsedEntry
	photoErr      error
	textResponse  []ai.ParsedEntry
	textErr       error
}

func (f *fakeAI) ParseEntry(_ context.Context, in ai.EntryInput) (ai.ParsedEntry, error) {
	f.requests = append(f.requests, in)
	if p, ok := f.responses[in.Message]; ok {
		return p, nil
	}
	return ai.ParsedEntry{}, fmt.Errorf("no canned response for %q", in.Message)
}

func (f *fakeAI) ParsePhotoReceipt(_ context.Context, _ []byte, _ ai.ReceiptInput) ([]ai.ParsedEntry, error) {
	if f.photoErr != nil {
		return nil, f.photoErr
	}
	return f.photoResponse, nil
}

func (f *fakeAI) ParseTextReceipt(_ context.Context, _ string, _ ai.ReceiptInput) ([]ai.ParsedEntry, error) {
	if f.textErr != nil {
		return nil, f.textErr
	}
	return f.textResponse, nil
}

func (f *fakeAI) ParseRecapFilter(_ context.Context, text string, _ time.Time, _ []string, _ string) (ai.RecapFilter, error) {
	f.recapRequests = append(f.recapRequests, text)
	if r, ok := f.recapResponses[text]; ok {
		return r, nil
	}
	return ai.RecapFilter{}, fmt.Errorf("no canned recap response for %q", text)
}

type fakeSender struct {
	messages  []string
	documents []string // filenames sent via SendDocument
	lastDoc   []byte
}

func (f *fakeSender) SendDocument(_ int64, filename string, data []byte) error {
	f.documents = append(f.documents, filename)
	f.lastDoc = data
	return nil
}

func (f *fakeSender) SendMessage(_ int64, text string) error {
	f.messages = append(f.messages, text)
	return nil
}

func (f *fakeSender) SendMessages(_ int64, parts []string) error {
	f.messages = append(f.messages, parts...)
	return nil
}

// --- helpers ---

var fixedNow = time.Date(2026, 10, 18, 15, 0, 0, 0, time.UTC)

type fakeDownloader struct {
	data    []byte
	err     error
	fileIDs []string
}

func (f *fakeDownloader) Download(_ context.Context, fileID string) ([]byte, error) {
	f.fileIDs = append(f.fileIDs, fileID)
	return f.data, f.err
}

func newTestDispatcher(fs *fakeStore, fai *fakeAI, send *fakeSender) *Dispatcher {
	authorizer := auth.New(fs, []string{"Rifi Anggriawan"})
	return NewDispatcher(fs, authorizer, fai, send, fdown, time.UTC, "MYR", func() time.Time { return fixedNow })
}

// fdown is the package-default fake downloader; receipt tests may swap it.
var fdown = &fakeDownloader{data: []byte("jpeg-bytes")}

// --- flow tests ---

func TestEntryFlow(t *testing.T) {
	tests := []struct {
		name string
		text string
		ai   ai.ParsedEntry
		// expected insert payload
		wantAmount string
		wantCur    string
		wantDate   string
		wantDir    string
		wantReply  string
	}{
		{
			name: "simple today", text: "10rm nasi lemak",
			ai:         ai.ParsedEntry{IsTransaction: true, Item: "nasi lemak", Category: "Meals", Direction: "expense", Amount: 10, Currency: "MYR", Date: "2026-10-18"},
			wantAmount: "10.00", wantCur: "MYR", wantDate: "2026-10-18", wantDir: "expense",
			wantReply: "🆔: 1\n🛍️: nasi lemak\n💵: RM 10.00\n🗓️: 2026-10-18\n🔠: Meals\nThanks, Rifi!",
		},
		{
			name: "yesterday", text: "nasi lemak, 10rm yesterday",
			ai:         ai.ParsedEntry{IsTransaction: true, Item: "nasi lemak", Category: "Meals", Direction: "expense", Amount: 10, Currency: "MYR", Date: "2026-10-17"},
			wantAmount: "10.00", wantCur: "MYR", wantDate: "2026-10-17", wantDir: "expense",
			wantReply: "🆔: 2\n🛍️: nasi lemak\n💵: RM 10.00\n🗓️: 2026-10-17\n🔠: Meals\nThanks, Rifi!",
		},
		{
			name: "idr kemarin", text: "rp50k bakso kemarin",
			ai:         ai.ParsedEntry{IsTransaction: true, Item: "bakso", Category: "Meals", Direction: "expense", Amount: 50000, Currency: "IDR", Date: "2026-10-17"},
			wantAmount: "50000", wantCur: "IDR", wantDate: "2026-10-17", wantDir: "expense",
			wantReply: "🆔: 3\n🛍️: bakso\n💵: Rp 50.000\n🗓️: 2026-10-17\n🔠: Meals\nTerima kasih, Rifi!",
		},
		{
			name: "income", text: "gaji 6500",
			ai:         ai.ParsedEntry{IsTransaction: true, Item: "gaji", Category: "Others", Direction: "income", Amount: 6500, Currency: "MYR", Date: "2026-10-18"},
			wantAmount: "6500.00", wantCur: "MYR", wantDate: "2026-10-18", wantDir: "income",
			wantReply: "🆔: 4\n🛍️: gaji\n💵: RM 6,500.00\n🗓️: 2026-10-18\n🔠: Others\n↩️: Income\nTerima kasih, Rifi!",
		},
		{
			name: "not a transaction", text: "hello there",
			ai:        ai.ParsedEntry{IsTransaction: false},
			wantReply: "That doesn't look like a transaction. Try something like '10rm nasi lemak' or 'salary 5000 this month'.",
		},
	}

	responses := map[string]ai.ParsedEntry{}
	for _, tt := range tests {
		responses[tt.text] = tt.ai
	}

	fs := &fakeStore{}
	fai := &fakeAI{responses: responses}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	for i, tt := range tests {
		d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan", router.Message{Text: tt.text})

		reply := send.messages[len(send.messages)-1]
		if reply != tt.wantReply {
			t.Errorf("%s: reply =\n%q\nwant\n%q", tt.name, reply, tt.wantReply)
		}
		if !tt.ai.IsTransaction {
			continue
		}
		got := fs.inserted[i]
		if got.TotalAmount != tt.wantAmount || got.CurrencyCode != tt.wantCur ||
			got.EntryDate.Format("2006-01-02") != tt.wantDate || got.Direction != tt.wantDir {
			t.Errorf("%s: insert = %+v", tt.name, got)
		}
		if got.Source != "text" || got.RawMessage != tt.text {
			t.Errorf("%s: source/raw = %q/%q", tt.name, got.Source, got.RawMessage)
		}
	}

	if len(fai.requests) != 5 {
		t.Errorf("AI called %d times, want 5", len(fai.requests))
	}
}

func TestNoEntryParseForOtherIntents(t *testing.T) {
	fs := &fakeStore{rows: []store.Transaction{{ID: 5, UserID: 55, Item: "x"}}}
	fai := &fakeAI{
		responses:      map[string]ai.ParsedEntry{},
		recapResponses: map[string]ai.RecapFilter{},
	}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	for _, m := range []router.Message{
		{Text: "delete 5"}, {Text: "photo", HasPhoto: true}, {DocumentName: "s.pdf"},
	} {
		d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan", m)
	}
	if len(fai.requests) != 0 {
		t.Errorf("ParseEntry called %d times for non-entry intents, want 0", len(fai.requests))
	}
	if len(fai.recapRequests) != 0 {
		t.Errorf("ParseRecapFilter called %d times for non-recap intents, want 0", len(fai.recapRequests))
	}
}

func TestValidationFailures(t *testing.T) {
	tests := []struct {
		name string
		text string
		ai   ai.ParsedEntry
	}{
		{"bad category", "x", ai.ParsedEntry{IsTransaction: true, Item: "x", Category: "Nope", Amount: 5, Currency: "MYR", Date: "2026-10-18"}},
		{"bad currency", "x", ai.ParsedEntry{IsTransaction: true, Item: "x", Category: "Meals", Amount: 5, Currency: "EUR", Date: "2026-10-18"}},
		{"zero amount", "x", ai.ParsedEntry{IsTransaction: true, Item: "x", Category: "Meals", Amount: 0, Currency: "MYR", Date: "2026-10-18"}},
		{"future date", "x", ai.ParsedEntry{IsTransaction: true, Item: "x", Category: "Meals", Amount: 5, Currency: "MYR", Date: "2026-10-25"}},
	}
	for _, tt := range tests {
		fs := &fakeStore{}
		fai := &fakeAI{responses: map[string]ai.ParsedEntry{tt.text: tt.ai}}
		send := &fakeSender{}
		d := newTestDispatcher(fs, fai, send)
		d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan", router.Message{Text: tt.text})

		if len(fs.inserted) != 0 {
			t.Errorf("%s: row inserted despite validation failure", tt.name)
		}
		if got := send.messages[len(send.messages)-1]; got != parseFailHint {
			t.Errorf("%s: reply = %q, want parse-fail hint", tt.name, got)
		}
	}
}

func TestUnauthorizedDropped(t *testing.T) {
	fs := &fakeStore{unauthorized: true}
	fai := &fakeAI{responses: map[string]ai.ParsedEntry{}}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	d.Handle(context.Background(), 1, 55, 666, "Stranger", "Danger", router.Message{Text: "10rm nasi lemak"})
	if len(send.messages) != 0 {
		t.Errorf("unauthorized sender got replies: %v", send.messages)
	}
	if len(fai.requests) != 0 {
		t.Error("AI called for unauthorized sender")
	}
	if len(fs.ensured) != 0 {
		t.Error("unauthorized sender was persisted")
	}
}

func TestFormatAmount(t *testing.T) {
	myr := &store.Currency{Code: "MYR", Symbol: "RM", DecimalPlaces: 2}
	idr := &store.Currency{Code: "IDR", Symbol: "Rp", DecimalPlaces: 0}
	usd := &store.Currency{Code: "USD", Symbol: "$", DecimalPlaces: 2}
	tests := []struct {
		c    *store.Currency
		in   string
		want string
	}{
		{myr, "10", "RM 10.00"},
		{myr, "10.5", "RM 10.50"},
		{myr, "1234.56", "RM 1,234.56"},
		{idr, "50000", "Rp 50.000"},
		{idr, "150000", "Rp 150.000"},
		{idr, "500", "Rp 500"},
		{usd, "0.99", "$ 0.99"},
	}
	for _, tt := range tests {
		if got := FormatAmount(tt.c, tt.in); got != tt.want {
			t.Errorf("FormatAmount(%s, %q) = %q, want %q", tt.c.Code, tt.in, got, tt.want)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	for _, id := range []string{"rekap bulan ini", "gaji sudah masuk", "kemarin makan bakso"} {
		if detectLanguage(id) != "ID" {
			t.Errorf("detectLanguage(%q) != ID", id)
		}
	}
	for _, en := range []string{"lunch yesterday", "hello there", "10rm nasi lemak"} {
		if detectLanguage(en) != "EN" {
			t.Errorf("detectLanguage(%q) != EN", en)
		}
	}
}
