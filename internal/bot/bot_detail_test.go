package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func TestRouterClassifiesDetail(t *testing.T) {
	for _, c := range []struct {
		in string
		id int64
	}{{"detail 5", 5}, {"rincian #7", 7}, {"info 12", 12}} {
		got := router.Classify(router.Message{Text: c.in})
		if got.Kind != router.IntentDetail || got.EditID != c.id {
			t.Errorf("%q => %v id=%d", c.in, got.Kind, got.EditID)
		}
	}
	if got := router.Classify(router.Message{Text: "detail"}); got.Kind == router.IntentDetail {
		t.Error("bare detail must not match")
	}
}

func detailSetup() (*fakeStore, *fakeSender, *Dispatcher) {
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 7, UserID: 55, Item: "Giant run", CategoryName: "Groceries", CurrencyCode: "MYR",
			TotalAmount: "9.88", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC),
			Source: "photo", RawMessage: "receipt.jpg",
			CreatedAt: time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)},
	}}
	sender := &fakeSender{}
	return fs, sender, newTestDispatcher(fs, &fakeAI{}, sender)
}

func TestHandleDetailTable(t *testing.T) {
	_, sender, d := detailSetup()
	out := d.handleDetail(context.Background(), 1, store.User{ID: 55}, 7)
	if out != "detail" {
		t.Fatalf("outcome=%s msgs=%v md=%v", out, sender.messages, sender.markdowns)
	}
	if len(sender.markdowns) != 1 {
		t.Fatalf("markdown not sent: %v", sender.markdowns)
	}
	body := sender.markdowns[0]
	for _, want := range []string{"Giant run", "Groceries", "```", "MAMEE", "voucher -1", "receipt.jpg"} {
		if !strings.Contains(body, want) {
			t.Errorf("detail missing %q:\n%s", want, body)
		}
	}
	// Header row present with a dashes separator under it.
	haveHeader, haveDashes := false, false
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "Item") && strings.Contains(l, "Amount") {
			haveHeader = true
		}
		if len(l) > 5 && strings.Trim(l, "-") == "" {
			haveDashes = true
		}
	}
	if !haveHeader || !haveDashes {
		t.Errorf("table structure missing (header=%v dashes=%v):\n%s", haveHeader, haveDashes, body)
	}
}

func TestHandleDetailNotFound(t *testing.T) {
	_, sender, d := detailSetup()
	out := d.handleDetail(context.Background(), 1, store.User{ID: 55}, 99)
	if out != "not_found" || sender.messages[len(sender.messages)-1] != "Record not found!" {
		t.Errorf("want generic not-found, got %s / %v", out, sender.messages)
	}
	// Other user's row -> same generic reply.
	out = d.handleDetail(context.Background(), 1, store.User{ID: 66}, 7)
	if out != "not_found" {
		t.Errorf("cross-user leak: outcome=%s", out)
	}
}
