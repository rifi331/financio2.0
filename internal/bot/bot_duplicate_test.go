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
