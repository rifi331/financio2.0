package bot

import (
	"context"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func TestRouterClassifiesEdit(t *testing.T) {
	for _, c := range []struct {
		in   string
		want bool
		id   int64
	}{
		{"edit 123 amount 12.50", true, 123},
		{"ubah #5 category Snacks", true, 5},
		{"edit", false, 0},
		{"edit 123", false, 0},
		{"the edit 5 thing", false, 0},
	} {
		got := router.Classify(router.Message{Text: c.in})
		if (got.Kind == router.IntentEdit) != c.want {
			t.Errorf("%q classify = %v", c.in, got.Kind)
		}
		if c.want && got.EditID != c.id {
			t.Errorf("%q id = %d", c.in, got.EditID)
		}
	}
}

func editSetup() (*fakeStore, *fakeSender, *Dispatcher) {
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 7, UserID: 55, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR",
			TotalAmount: "10.50", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
	}}
	sender := &fakeSender{}
	return fs, sender, newTestDispatcher(fs, &fakeAI{}, sender)
}

func TestEditAmount(t *testing.T) {
	fs, sender, d := editSetup()
	out := d.handleEdit(context.Background(), 1, store.User{ID: 55}, 7, "amount 12.50")
	if out != "edited" {
		t.Errorf("outcome=%s msgs=%v", out, sender.messages)
	}
	if fs.lastPatch == nil || fs.lastPatch.Amount == nil || *fs.lastPatch.Amount != "12.50" {
		t.Errorf("patch amount wrong: %+v", fs.lastPatch)
	}
}

func TestEditMultipleFields(t *testing.T) {
	fs, sender, d := editSetup()
	d.handleEdit(context.Background(), 1, store.User{ID: 55}, 7, "category Snacks date 2026-10-01")
	if fs.lastPatch == nil || fs.lastPatch.Category == nil || *fs.lastPatch.Category != "Snacks" ||
		fs.lastPatch.Date == nil || *fs.lastPatch.Date != "2026-10-01" {
		t.Errorf("multi-field patch wrong: %+v", fs.lastPatch)
	}
	if fs.lastPatch.Amount != nil {
		t.Error("amount must be untouched")
	}
	_ = sender
}

func TestEditNotFoundGeneric(t *testing.T) {
	_, sender, d := editSetup()
	out := d.handleEdit(context.Background(), 1, store.User{ID: 55}, 99, "amount 12.50")
	if out != "not_found" || sender.messages[len(sender.messages)-1] != "Record not found!" {
		t.Errorf("want generic not-found, got %s / %v", out, sender.messages)
	}
}

func TestEditInvalidValues(t *testing.T) {
	_, sender, d := editSetup()
	for _, spec := range []string{"amount -5", "amount abc", "date 31-12-2026", "category Nope", "currency EUR", "bogus"} {
		n := len(sender.messages)
		out := d.handleEdit(context.Background(), 1, store.User{ID: 55}, 7, spec)
		if out != "invalid" || len(sender.messages) == n {
			t.Errorf("%q should be rejected (got %s)", spec, out)
		}
	}
}
