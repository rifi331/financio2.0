package bot

import (
	"context"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func deleteTestDispatcher(fs *fakeStore, fai *fakeAI, send *fakeSender) *Dispatcher {
	return newTestDispatcher(fs, fai, send)
}

func TestDeleteSuccessEN(t *testing.T) {
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 7, UserID: 55, Item: "nasi lemak"},
	}}
	send := &fakeSender{}
	d := deleteTestDispatcher(fs, &fakeAI{}, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "delete 7"})

	want := "🗑️ item deleted for id: 7\n🛍️: nasi lemak"
	if got := send.messages[0]; got != want {
		t.Errorf("reply = %q, want %q", got, want)
	}
	if len(fs.rows) != 0 {
		t.Error("row not deleted")
	}
}

func TestDeleteSuccessID(t *testing.T) {
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 3, UserID: 55, Item: "bakso"},
	}}
	send := &fakeSender{}
	d := deleteTestDispatcher(fs, &fakeAI{}, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "hapus #3"})

	want := "🗑️ item dengan id 3 sudah dihapus\n🛍️: bakso"
	if got := send.messages[0]; got != want {
		t.Errorf("reply = %q, want %q", got, want)
	}
}

func TestDeleteNotFound(t *testing.T) {
	fs := &fakeStore{}
	send := &fakeSender{}
	d := deleteTestDispatcher(fs, &fakeAI{}, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "delete 99"})

	if got := send.messages[0]; got != "Record not found!" {
		t.Errorf("reply = %q, want Record not found!", got)
	}
}

func TestDeleteNotOwnedIsRecordNotFound(t *testing.T) {
	// Row 7 belongs to user 999. The requester must see the same generic
	// "not found" message and the row must survive.
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 7, UserID: 999, Item: "someone else's secret"},
	}}
	send := &fakeSender{}
	d := deleteTestDispatcher(fs, &fakeAI{}, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "del 7"})

	if got := send.messages[0]; got != "Record not found!" {
		t.Errorf("reply = %q, want Record not found!", got)
	}
	if len(fs.rows) != 1 {
		t.Error("other user's row was deleted")
	}
}

func TestDeleteNotFoundID(t *testing.T) {
	fs := &fakeStore{}
	send := &fakeSender{}
	d := deleteTestDispatcher(fs, &fakeAI{}, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "hapus 12"})

	if got := send.messages[0]; got != "Catatan tidak ditemukan!" {
		t.Errorf("reply = %q, want Catatan tidak ditemukan!", got)
	}
}

// --- recap ---

func seedTxns(uid int64) []store.Transaction {
	day := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	return []store.Transaction{
		{ID: 1, UserID: uid, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "10.00", EntryDate: day},
		{ID: 2, UserID: uid, Item: "tol", CategoryName: "Public Transportation", CurrencyCode: "IDR", TotalAmount: "150000", EntryDate: day},
		{ID: 3, UserID: uid, Item: "ayam", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "20.00", EntryDate: day.AddDate(0, 0, 1)},
	}
}

func TestRecapFlow(t *testing.T) {
	fs := &fakeStore{rows: seedTxns(55)}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"recap october": {
			IsRecapRequest: true,
			DateFrom:       "2026-10-01", DateTo: "2026-10-31",
			Categories: []string{"meals"}, // case-insensitive intersect -> "Meals"
			Language:   "EN",
		},
	}}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "recap october"})

	if len(send.messages) != 1 {
		t.Fatalf("messages = %d, want 1: %q", len(send.messages), send.messages)
	}
	want := "📅 Summary 2026-10-01 - 2026-10-31\n" +
		"- 🗓️ 2026-10-17:\n" +
		"  - 🆔: 1, 🛍️: nasi lemak, 💵: RM 10.00\n" +
		"- 🗓️ 2026-10-18:\n" +
		"  - 🆔: 3, 🛍️: ayam, 💵: RM 20.00\n" +
		"💰 Total: MYR 30.00\n" +
		"🔠 by category: Meals MYR 30.00"
	if got := send.messages[0]; got != want {
		t.Errorf("reply =\n%q\nwant\n%q", got, want)
	}
	// The store filter must be typed values only — user text never enters SQL.
	if fs.lastFilter == nil || fs.lastFilter.UserID != 55 ||
		fs.lastFilter.Direction != "expense" || // recap is spending-only
		len(fs.lastFilter.Categories) != 1 || fs.lastFilter.Categories[0] != "Meals" {
		t.Errorf("filter = %+v", fs.lastFilter)
	}
}

func TestRecapEmpty(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"rekap bulan ini": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-10-31", Language: "ID"},
	}}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "rekap bulan ini"})

	if got := send.messages[0]; got != "Tidak ada catatan pada periode ini." {
		t.Errorf("reply = %q", got)
	}
}

func TestRecapInvalidRange(t *testing.T) {
	fs := &fakeStore{rows: seedTxns(55)}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"recap weird": {IsRecapRequest: true, DateFrom: "2026-10-31", DateTo: "2026-10-01", Language: "EN"},
	}}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "recap weird"})

	if len(send.messages) != 1 || send.messages[0] == "" {
		t.Fatalf("expected one error reply, got %q", send.messages)
	}
	if send.messages[0] != "Sorry, I couldn't process that recap request. Try e.g. 'recap nov 2025' or 'rekap bulan ini'." {
		t.Errorf("reply = %q", send.messages[0])
	}
}

func TestRecapNotRecapRequest(t *testing.T) {
	fs := &fakeStore{rows: seedTxns(55)}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"recap nov 2025": {IsRecapRequest: false},
	}}
	send := &fakeSender{}
	d := newTestDispatcher(fs, fai, send)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "recap nov 2025"})

	if len(fai.recapRequests) != 1 {
		t.Fatalf("ParseRecapFilter calls = %d", len(fai.recapRequests))
	}
	if len(send.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(send.messages))
	}
}

func TestRecapRangeClampedTo5Years(t *testing.T) {
	// 10-year range gets clamped to 5 years from date_from.
	from, to, err := parseAndValidateRange("2000-01-01", "2010-01-01", fixedNow)
	if err != nil {
		t.Fatalf("parseAndValidateRange: %v", err)
	}
	if got := to.Sub(from) / (365 * 24 * time.Hour); got != 5 {
		t.Errorf("range = %v years, want 5", got)
	}

	if _, _, err := parseAndValidateRange("", "2026-01-01", fixedNow); err == nil {
		t.Error("empty date_from should error")
	}
	if _, _, err := parseAndValidateRange("2026-01-01", "bad", fixedNow); err == nil {
		t.Error("bad date_to should error")
	}
}
