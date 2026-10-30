package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/store"
)

func alertSetup() (*fakeStore, *fakeSender, *Dispatcher) {
	fs := &fakeStore{
		userGroup:        1,
		budgets:          map[[2]string]string{{"Meals", "MYR"}: "500.00"},
		budgetGroupOwner: map[[2]string]int64{{"Meals", "MYR"}: 1},
		rows: []store.Transaction{
			{ID: 1, UserID: 55, Item: "a", CategoryName: "Meals", CurrencyCode: "MYR",
				TotalAmount: "450.00", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		},
		alerted: map[string]bool{},
	}
	sender := &fakeSender{}
	return fs, sender, newTestDispatcher(fs, &fakeAI{}, sender)
}

func TestAlertFiresAt80Once(t *testing.T) {
	_, sender, d := alertSetup()
	now := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	if n := d.RunAlerts(context.Background(), now); n != 1 {
		t.Fatalf("alerts fired %d, want 1", n)
	}
	if !strings.Contains(sender.messages[0], "Meals") || !strings.Contains(sender.messages[0], "90%") {
		t.Errorf("alert text wrong: %q", sender.messages[0])
	}
	if n := d.RunAlerts(context.Background(), now.Add(time.Hour)); n != 0 {
		t.Errorf("alert must fire once per month, fired %d more", n)
	}
}

func TestAlertBelow80Silent(t *testing.T) {
	fs := &fakeStore{
		userGroup: 1, budgets: map[[2]string]string{{"Meals", "MYR"}: "500.00"},
		budgetGroupOwner: map[[2]string]int64{{"Meals", "MYR"}: 1}, alerted: map[string]bool{},
		rows: []store.Transaction{
			{ID: 1, UserID: 55, Item: "a", CategoryName: "Meals", CurrencyCode: "MYR",
				TotalAmount: "100.00", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		},
	}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	if n := d.RunAlerts(context.Background(), time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)); n != 0 {
		t.Fatalf("20%% spend must not alert, fired %d", n)
	}
}
