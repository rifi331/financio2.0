package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func TestRouterClassifiesRecurring(t *testing.T) {
	for _, in := range []string{"/recurring", "/recurring list", "/recurring add Rent 1200 day 1"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentRecurring {
			t.Errorf("%q => %v", in, got.Kind)
		}
	}
	if got := router.Classify(router.Message{Text: "recurring thing"}); got.Kind == router.IntentRecurring {
		t.Error("plain text must not be recurring")
	}
}

func recSetup() (*fakeStore, *fakeSender, *Dispatcher) {
	fs := &fakeStore{}
	sender := &fakeSender{}
	return fs, sender, newTestDispatcher(fs, &fakeAI{}, sender)
}

func TestRecurringAddListDelete(t *testing.T) {
	fs, sender, d := recSetup()
	ctx := context.Background()

	d.handleRecurring(ctx, 1, store.User{ID: 55}, "/recurring add Rent 1200 day 1 category Housing")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Rent") || !strings.Contains(last, "day 1") {
		t.Fatalf("add reply: %q", last)
	}
	if len(fs.recurring) != 1 || fs.recurring[0].CategoryName != "Housing" || fs.recurring[0].Amount != "1200.00" {
		t.Errorf("stored rule wrong: %+v", fs.recurring)
	}

	d.handleRecurring(ctx, 2, store.User{ID: 55}, "/recurring list")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "#1") || !strings.Contains(last, "Rent") {
		t.Errorf("list reply: %q", last)
	}

	d.handleRecurring(ctx, 3, store.User{ID: 55}, "/recurring delete 1")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "removed") {
		t.Errorf("delete reply: %q", last)
	}
	if len(fs.recurring) != 0 {
		t.Error("rule not deleted")
	}
}

func TestRunDueInsertsAndConfirms(t *testing.T) {
	fs, sender, d := recSetup()
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	fs.recurring = []store.Recurring{{
		ID: 1, UserID: 55, Item: "Rent", CategoryName: "Housing", CurrencyCode: "MYR",
		Amount: "1200.00", Direction: "expense", DayOfMonth: 1, IsActive: true,
	}}

	n := d.RunDue(context.Background(), now)

	if n != 1 {
		t.Fatalf("inserted %d, want 1", n)
	}
	if len(fs.inserted) != 1 || fs.inserted[0].Item != "Rent" || fs.inserted[0].Source != "recurring" {
		t.Errorf("insert payload wrong: %+v", fs.inserted)
	}
	if fs.recurring[0].LastRun == nil || fs.recurring[0].LastRun.Format("2006-01") != "2026-10" {
		t.Error("last_run not marked")
	}
	if len(sender.messages) == 0 || !strings.Contains(sender.messages[0], "Rent") {
		t.Errorf("owner confirmation missing: %v", sender.messages)
	}

	// Second run in the same month: idempotent.
	if n := d.RunDue(context.Background(), now); n != 0 {
		t.Errorf("second run inserted %d, want 0", n)
	}
}

func TestRunDueClampsDay31(t *testing.T) {
	fs, _, d := recSetup()
	now := time.Date(2026, 2, 28, 9, 0, 0, 0, time.UTC) // Feb: day 31 clamps to 28
	fs.recurring = []store.Recurring{{
		ID: 1, UserID: 55, Item: "Gym", CategoryName: "Others", CurrencyCode: "MYR",
		Amount: "100.00", Direction: "expense", DayOfMonth: 31, IsActive: true,
	}}
	if n := d.RunDue(context.Background(), time.Date(2026, 2, 10, 9, 0, 0, 0, time.UTC)); n != 0 {
		t.Errorf("must not run before the clamped day, got %d", n)
	}
	if n := d.RunDue(context.Background(), now); n != 1 {
		t.Fatalf("clamped rule should run on day 28, got %d", n)
	}
	if got := fs.inserted[0].EntryDate.Day(); got != 28 {
		t.Errorf("day clamped to %d, want 28", got)
	}
}

// Crash between insert and MarkRecurringRun must not double-post on retry.
func TestRunDueNoDoublePostAfterMissedMark(t *testing.T) {
	fs, _, d := recSetup()
	fs.recurring = []store.Recurring{{
		ID: 1, UserID: 55, Item: "Rent", CategoryName: "Housing", CurrencyCode: "MYR",
		Amount: "1200.00", Direction: "expense", DayOfMonth: 1, IsActive: true,
	}}
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	d.RunDue(context.Background(), now)

	fs.recurring[0].LastRun = nil // simulate the mark being lost
	if n := d.RunDue(context.Background(), now.Add(time.Hour)); n != 0 {
		t.Fatalf("re-ran despite existing entry: %d", n)
	}
	if len(fs.inserted) != 1 {
		t.Errorf("inserted %d, want 1", len(fs.inserted))
	}
}
