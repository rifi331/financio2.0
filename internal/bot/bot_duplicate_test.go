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

func TestRouterClassifiesDuplicate(t *testing.T) {
	for _, in := range []string{
		"check duplicate october", "check duplicates oct",
		"cek duplikat oktober", "find duplicate nov 2025", "cari duplicate",
	} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentDuplicate {
			t.Errorf("%q => %v, want duplicate", in, got.Kind)
		}
	}
	// Not duplicates:
	for _, in := range []string{"recap october", "delete 12", "10rm nasi lemak"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind == router.IntentDuplicate {
			t.Errorf("%q should not be duplicate", in)
		}
	}
}

func TestHandleDuplicateListsGroups(t *testing.T) {
	fs := &fakeStore{dupGroups: []store.DuplicateGroup{
		{Item: "nasi lemak", Amount: "10.50", Currency: "MYR",
			EntryDate: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), IDs: []int64{101, 102}},
	}}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"check duplicate october": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "EN"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 2, 55, "Rifi", "Anggriawan", router.Message{Text: "check duplicate october"})

	if len(sender.messages) == 0 {
		t.Fatal("no reply sent")
	}
	body := strings.Join(sender.messages, "\n")
	for _, want := range []string{
		"1 duplicate groups in October 2026",
		"2x MYR 10.50", "nasi lemak", "2026-10-05", "delete 101 / delete 102",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("reply missing %q\nbody: %s", want, body)
		}
	}
	if fs.lastDupRange == nil || fs.lastDupRange[0].Format("2006-01-02") != "2026-10-01" {
		t.Errorf("duplicate range wrong: %+v", fs.lastDupRange)
	}
}

func TestHandleDuplicateEmpty(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"cek duplikat oktober": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "ID"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)
	d.Handle(context.Background(), 1, 2, 55, "Rifi", "Anggriawan", router.Message{Text: "cek duplikat oktober"})
	body := strings.Join(sender.messages, "\n")
	if !strings.Contains(body, "Tidak ada transaksi duplikat pada October 2026") {
		t.Errorf("ID empty reply wrong: %q", body)
	}
}

func TestHandleDuplicateNotRecapRequest(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"check duplicate october": {IsRecapRequest: false, Language: "EN"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	out := d.handleDuplicate(context.Background(), 1, store.User{ID: 55}, "check duplicate october")

	if out != "error" {
		t.Errorf("outcome = %s, want error", out)
	}
	if fs.lastDupRange != nil {
		t.Error("store must not be queried when filter says not a recap request")
	}
	if len(sender.messages) == 0 || !strings.Contains(sender.messages[0], "couldn't process") {
		t.Errorf("friendly hint expected: %v", sender.messages)
	}
}

func dupGroupAt(mins int, id int64, src string) store.DuplicateGroup {
	return store.DuplicateGroup{
		Item: "mrt", Amount: "2.50", Currency: "MYR",
		EntryDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
		IDs:       []int64{id},
		Rows: []store.Transaction{{
			ID: id, Item: "mrt", CurrencyCode: "MYR", TotalAmount: "2.50",
			EntryDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC),
			Source:    src,
			CreatedAt: time.Date(2026, 10, 14, 8, mins, 0, 0, time.UTC),
		}},
	}
}

func TestDuplicateEnrichedRendering(t *testing.T) {
	fs, _, sender, d := dupTestSetup(t)
	// Accidental: two pdf rows 2 minutes apart.
	g := dupGroupAt(0, 101, "pdf")
	g.IDs = append(g.IDs, 102)
	g.Rows = append(g.Rows, dupGroupAt(2, 102, "pdf").Rows[0])
	fs.dupGroups = []store.DuplicateGroup{g}

	d.handleDuplicate(context.Background(), 1, store.User{ID: 55}, "check duplicate october")
	body := strings.Join(sender.messages, "\n")
	for _, want := range []string{"likely accidental", "#101 pdf", "#102 pdf", "08:02"} {
		if !strings.Contains(body, want) {
			t.Errorf("enriched reply missing %q:\n%s", want, body)
		}
	}
}

func TestDuplicateSameDayPairHint(t *testing.T) {
	fs, _, sender, d := dupTestSetup(t)
	g := dupGroupAt(8*60, 201, "text") // 08:00
	g.IDs = append(g.IDs, 202)
	g.Rows = append(g.Rows, dupGroupAt(10*60, 202, "text").Rows[0]) // 10:00
	fs.dupGroups = []store.DuplicateGroup{g}

	d.handleDuplicate(context.Background(), 1, store.User{ID: 55}, "check duplicate october")
	body := strings.Join(sender.messages, "\n")
	if !strings.Contains(body, "same-day pair") {
		t.Errorf("legit pair should hint same-day:\n%s", body)
	}
	if strings.Contains(body, "likely accidental") {
		t.Error("text rows hours apart must not be flagged accidental")
	}
}

func dupTestSetup(t *testing.T) (*fakeStore, *fakeAI, *fakeSender, *Dispatcher) {
	t.Helper()
	fs := &fakeStore{}
	fai := &fakeAI{recapResponses: map[string]ai.RecapFilter{
		"check duplicate october": {IsRecapRequest: true, DateFrom: "2026-10-01", DateTo: "2026-11-01", Language: "EN"},
	}}
	sender := &fakeSender{}
	return fs, fai, sender, newTestDispatcher(fs, fai, sender)
}
