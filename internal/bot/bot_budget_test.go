package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func TestRouterClassifiesBudget(t *testing.T) {
	for _, in := range []string{"/budget", "/budget list", "/budget set Meals 500"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentBudget {
			t.Errorf("%q => %v, want budget", in, got.Kind)
		}
	}
}

func TestBudgetSetListDelete(t *testing.T) {
	fs := &fakeStore{}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()

	d.Handle(ctx, 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/budget set Meals 500"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Meals budget is MYR 500/month") {
		t.Errorf("set reply: %q", last)
	}

	d.Handle(ctx, 2, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/budget list"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Meals: MYR 500.00/month") {
		t.Errorf("list reply: %q", last)
	}

	d.Handle(ctx, 3, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/budget delete Meals"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "removed") {
		t.Errorf("delete reply: %q", last)
	}
	if len(fs.budgets) != 0 {
		t.Errorf("budget not deleted: %v", fs.budgets)
	}
}

// Entry into a budgeted category appends the expense/budget line.
func TestEntryReplyIncludesBudgetLine(t *testing.T) {
	fs := &fakeStore{budgets: map[[2]string]string{{"Meals", "MYR"}: "500.00"}}
	fai := &fakeAI{responses: map[string]ai.ParsedEntry{
		"10rm nasi lemak": {IsTransaction: true, Item: "nasi lemak", Category: "Meals", Amount: 10, Currency: "MYR", Date: "2026-10-18"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "10rm nasi lemak"})

	last := sender.messages[len(sender.messages)-1]
	if !strings.Contains(last, "Meals: MYR 10.00/MYR 500.00 (2%)") {
		t.Errorf("budget line missing/wrong: %q", last)
	}
}

// No budget -> no budget line.
func TestEntryReplyNoBudgetNoLine(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{responses: map[string]ai.ParsedEntry{
		"10rm nasi lemak": {IsTransaction: true, Item: "nasi lemak", Category: "Meals", Amount: 10, Currency: "MYR", Date: "2026-10-18"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "10rm nasi lemak"})

	if strings.Contains(sender.messages[len(sender.messages)-1], "MYR 500") {
		t.Error("unexpected budget line")
	}
}

// Percentage rounds half up: 10/200 = 5%, 10.50/200 = 5.25% -> 5%; 11/200 = 5.5% -> 6%.
func TestBudgetLineRounding(t *testing.T) {
	if got := budgetLine("Meals", "MYR", "11.00", "200.00"); !strings.Contains(got, "(6%)") {
		t.Errorf("5.5%% should round to 6%%: %q", got)
	}
	if got := budgetLine("Meals", "MYR", "10.00", "200.00"); !strings.Contains(got, "(5%)") {
		t.Errorf("exact 5%%: %q", got)
	}
	if got := budgetLine("Tol", "IDR", "150000", "1000000"); !strings.Contains(got, "IDR 150.000/IDR 1.000.000") {
		t.Errorf("IDR formatting: %q", got)
	}
}

// Recap appends budget lines as an extra part.
func TestRecapIncludesBudgetLine(t *testing.T) {
	fs := &fakeStore{
		rows: []store.Transaction{
			{ID: 1, UserID: 55, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR",
				TotalAmount: "242.75", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		},
		budgets: map[[2]string]string{{"Meals", "MYR"}: "500.00"},
	}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"recap october": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "EN"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "recap october"})

	joined := strings.Join(sender.messages, "\n---\n")
	if !strings.Contains(joined, "Meals: MYR 242.75/MYR 500.00 (49%)") {
		t.Errorf("recap budget line missing:\n%s", joined)
	}
}
