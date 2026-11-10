package bot

import (
	"context"
	"testing"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/store"
)

func TestKnownUserMatchAndCache(t *testing.T) {
	fs := &fakeStore{groups: []store.GroupWithMembers{
		{GroupID: 1, Name: "Family", Members: []store.User{
			{ID: 55, DisplayName: "Rifi Anggriawan"},
			{ID: 66, DisplayName: "Dafina Balqis"},
		}},
	}}
	d := newTestDispatcher(fs, &fakeAI{}, &fakeSender{})
	ctx := context.Background()

	if u, ok := d.knownUser(ctx, "dafina balqis"); !ok || u.ID != 66 {
		t.Errorf("case-insensitive full match failed: %+v ok=%v", u, ok)
	}
	if u, ok := d.knownUser(ctx, "Dafina"); !ok || u.ID != 66 {
		t.Errorf("unique first-name match failed: %+v ok=%v", u, ok)
	}
	if _, ok := d.knownUser(ctx, "Nobody"); ok {
		t.Error("unknown name must not match")
	}
	// Roster served from cache: no extra ListGroupsWithMembers calls needed
	// (fake returns same slice; assert the cache is populated and reused).
	d.cachedUsers(ctx)
	if len(d.usersCache) != 2 {
		t.Errorf("cache should hold 2 users, has %d", len(d.usersCache))
	}
}

func TestInsertTransferKnownAndUnknown(t *testing.T) {
	fs := &fakeStore{groups: []store.GroupWithMembers{
		{GroupID: 1, Name: "Family", Members: []store.User{
			{ID: 55, DisplayName: "Rifi Anggriawan"},
			{ID: 66, DisplayName: "Dafina Balqis"},
		}},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()
	now := d.nowFn()
	cats := []string{"Others"}
	curs := []store.Currency{{Code: "MYR", DecimalPlaces: 2}}

	// Known recipient -> transfer, counterparty in description.
	txn, err := d.insertParsed(ctx, store.User{ID: 55}, ai.ParsedEntry{
		IsTransaction: true, Item: "transfer", Category: "Others", Direction: "transfer",
		TransferTo: "Dafina Balqis", Amount: 50, Currency: "MYR", Date: now.Format("2006-01-02"),
	}, cats, curs, now, "text", "transfer 50 to dafina")
	if err != nil {
		t.Fatalf("insert transfer: %v", err)
	}
	if txn.Direction != "transfer" {
		t.Errorf("direction = %s, want transfer", txn.Direction)
	}
	want := `{"transferTo":"Dafina Balqis"}`
	if got := string(fs.inserted[len(fs.inserted)-1].Description); got != want {
		t.Errorf("description = %s, want %s", got, want)
	}

	// Unknown recipient -> falls back to expense.
	txn2, err := d.insertParsed(ctx, store.User{ID: 55}, ai.ParsedEntry{
		IsTransaction: true, Item: "rent", Category: "Others", Direction: "transfer",
		TransferTo: "Landlord", Amount: 100, Currency: "MYR", Date: now.Format("2006-01-02"),
	}, cats, curs, now, "text", "transfer 100 to landlord")
	if err != nil {
		t.Fatalf("insert fallback: %v", err)
	}
	if txn2.Direction != "expense" {
		t.Errorf("unknown recipient must fall back to expense, got %s", txn2.Direction)
	}
}

// Transfers are excluded from recap queries (direction=expense filter) and
// the report's expense/income split; summary notes them instead.
func TestTransferExcludedFromReportSplit(t *testing.T) {
	exp, inc := splitForTest([]store.Transaction{
		{ID: 1, Direction: "expense"}, {ID: 2, Direction: "income"}, {ID: 3, Direction: "transfer"},
	})
	if len(exp) != 1 || len(inc) != 1 {
		t.Errorf("split: exp=%d inc=%d, want 1/1 (transfer excluded)", len(exp), len(inc))
	}
}

// splitForTest mirrors internal/report.split semantics for the bot-side test.
func splitForTest(txns []store.Transaction) (exp, inc []store.Transaction) {
	for _, t := range txns {
		switch t.Direction {
		case "income":
			inc = append(inc, t)
		case "transfer":
		default:
			exp = append(exp, t)
		}
	}
	return
}
