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

func TestRouterClassifiesReport(t *testing.T) {
	for _, in := range []string{"full report oct 2026", "full report last month", "complete report nov 2025 meals", "laporan lengkap oktober"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentReport {
			t.Errorf("%q => %v, want report", in, got.Kind)
		}
	}
	for _, in := range []string{"recap oct", "check duplicate oct", "delete 1"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind == router.IntentReport {
			t.Errorf("%q should not be report", in)
		}
	}
}

func TestHandleReportSendsPDF(t *testing.T) {
	fs := &fakeStore{rows: []store.Transaction{
		{ID: 1, UserID: 55, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR",
			TotalAmount: "10.50", Direction: "expense", EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		{ID: 2, UserID: 55, Item: "gaji", CategoryName: "Others", CurrencyCode: "MYR",
			TotalAmount: "6500", Direction: "income", EntryDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	}}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"full report october": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "EN"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "full report october"})

	if len(sender.documents) != 1 {
		t.Fatalf("documents sent = %d, want 1 (messages: %v)", len(sender.documents), sender.messages)
	}
	if sender.documents[0] != "financio-report-2026-10.pdf" {
		t.Errorf("filename = %s", sender.documents[0])
	}
	if len(sender.lastDoc) < 1000 {
		t.Errorf("pdf too small: %d bytes", len(sender.lastDoc))
	}
	if !strings.HasPrefix(string(sender.lastDoc[:5]), "%PDF") {
		t.Error("payload is not a PDF")
	}
	// Both directions queried.
	if fs.lastFilter == nil || fs.lastFilter.Direction != "" {
		t.Errorf("report must query both directions, filter=%+v", fs.lastFilter)
	}
}

func TestHandleReportEmpty(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"full report october": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "EN"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "full report october"})

	if len(sender.documents) != 0 {
		t.Error("no document expected for empty period")
	}
	if len(sender.messages) == 0 || !strings.Contains(sender.messages[0], "No transactions") {
		t.Errorf("empty reply wrong: %v", sender.messages)
	}
}
