package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/receipt/testpdf"
	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

// bigPDF builds a valid single-page PDF whose extracted text is about n bytes.
func bigPDF(t *testing.T, n int) []byte {
	t.Helper()
	return testpdf.Build(strings.Repeat("a", n))
}

func newReceiptDispatcher(fs *fakeStore, fai *fakeAI, send *fakeSender, down *fakeDownloader) *Dispatcher {
	d := newTestDispatcher(fs, fai, send)
	d.down = down
	return d
}

// TestPhotoFlowMultiRows: a statement-like photo produces one insert per row
// (per-row dates preserved, source="photo") and ONE compact reply.
func TestPhotoFlowMultiRows(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{photoResponse: []ai.ParsedEntry{
		{IsTransaction: true, Item: "Giant", Category: "Groceries", Direction: "expense", Amount: 16.78, Currency: "MYR", Date: "2026-10-14"},
		{IsTransaction: true, Item: "Tolls", Category: "Others", Direction: "expense", Amount: 2.30, Currency: "MYR", Date: "2026-10-15"},
	}}
	send := &fakeSender{}
	down := &fakeDownloader{data: []byte("img")}
	d := newReceiptDispatcher(fs, fai, send, down)

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{Text: "receipt", HasPhoto: true, PhotoFileID: "ph1"})

	if len(fs.inserted) != 2 {
		t.Fatalf("inserted %d rows, want 2", len(fs.inserted))
	}
	for i, wantDate := range []string{"2026-10-14", "2026-10-15"} {
		if got := fs.inserted[i]; got.Source != "photo" || got.EntryDate.Format("2006-01-02") != wantDate {
			t.Errorf("row %d = %+v (source/date)", i, got)
		}
	}
	if fs.inserted[0].RawMessage != "receipt" {
		t.Errorf("raw_message = %q, want caption", fs.inserted[0].RawMessage)
	}
	if len(down.fileIDs) != 1 || down.fileIDs[0] != "ph1" {
		t.Errorf("downloaded file ids = %v", down.fileIDs)
	}

	want := "✅ 2 transactions saved\n" +
		"🆔 1: Giant RM 16.78 (Groceries, 2026-10-14)\n" +
		"🆔 2: Tolls RM 2.30 (Others, 2026-10-15)\n" +
		"💰 Total added: MYR 19.08"
	if len(send.messages) != 1 || send.messages[0] != want {
		t.Errorf("reply =\n%q\nwant\n%q", send.messages, want)
	}
}

// TestPhotoFlowSingleReceipt: one transaction -> the normal confirmation.
func TestPhotoFlowSingleReceipt(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{photoResponse: []ai.ParsedEntry{
		{IsTransaction: true, Item: "Giant", Category: "Groceries", Direction: "expense", Amount: 16.78, Currency: "MYR", Date: "2026-10-14",
			Details: []ai.Detail{{Item: "rice", Amount: 10, Qty: 1}}},
	}}
	send := &fakeSender{}
	d := newReceiptDispatcher(fs, fai, send, &fakeDownloader{data: []byte("img")})

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{Text: "", HasPhoto: true, PhotoFileID: "ph1"})

	if len(fs.inserted) != 1 || fs.inserted[0].Source != "photo" {
		t.Fatalf("inserted = %+v", fs.inserted)
	}
	want := "🆔: 1\n🛍️: Giant\n💵: RM 16.78\n🗓️: 2026-10-14\n🔠: Groceries\nThanks, Rifi!"
	if len(send.messages) != 1 || send.messages[0] != want {
		t.Errorf("reply =\n%q\nwant\n%q", send.messages, want)
	}
}

// TestPDFFlowTextGuard: a real PDF whose extracted text exceeds 30KB gets the
// localized unreadable reply and no inserts and no AI call.
func TestPDFFlowTextGuard(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{}
	send := &fakeSender{}
	down := &fakeDownloader{data: bigPDF(t, 40*1024)}
	d := newReceiptDispatcher(fs, fai, send, down)

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{DocumentName: "statement.pdf", DocumentFileID: "f1"})

	if len(fs.inserted) != 0 {
		t.Errorf("inserted %d rows, want 0", len(fs.inserted))
	}
	if want := "PDF could not be read. Try sending it as a photo instead, or re-export the file."; len(send.messages) != 1 || send.messages[0] != want {
		t.Errorf("reply = %q, want %q", send.messages, want)
	}
}

// TestPDFFlowLocalisedUnreadable: Indonesian caption gets the ID reply.
func TestPDFFlowLocalisedUnreadable(t *testing.T) {
	send := &fakeSender{}
	down := &fakeDownloader{data: []byte("not a pdf")}
	d := newReceiptDispatcher(&fakeStore{}, &fakeAI{}, send, down)

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{Text: "rekap struk bulan ini", DocumentName: "s.pdf", DocumentFileID: "f1"})

	if want := "PDF tidak bisa dibaca. Coba kirim ulang sebagai foto, atau ekspor ulang filenya."; len(send.messages) != 1 || send.messages[0] != want {
		t.Errorf("reply = %q, want %q", send.messages, want)
	}
}

// TestPDFFlowSmallPDF: a small valid PDF goes through the text parser with
// source="pdf" and the document name as raw_message.
func TestPDFFlowSmallPDF(t *testing.T) {
	fs := &fakeStore{}
	fai := &fakeAI{textResponse: []ai.ParsedEntry{
		{IsTransaction: true, Item: "Duitnow QR Kopi", Category: "Meals", Direction: "expense", Amount: 12.00, Currency: "MYR", Date: "2026-10-15"},
	}}
	send := &fakeSender{}
	down := &fakeDownloader{data: bigPDF(t, 2*1024)}
	d := newReceiptDispatcher(fs, fai, send, down)

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{DocumentName: "stmt.pdf", DocumentFileID: "f1"})

	if len(fs.inserted) != 1 {
		t.Fatalf("inserted %d rows, want 1", len(fs.inserted))
	}
	if fs.inserted[0].Source != "pdf" || fs.inserted[0].RawMessage != "stmt.pdf" {
		t.Errorf("insert = %+v", fs.inserted[0])
	}
	if len(send.messages) != 1 || !strings.Contains(send.messages[0], "🆔: 1") {
		t.Errorf("reply = %q", send.messages)
	}
}

func TestDownloadFailureReply(t *testing.T) {
	send := &fakeSender{}
	down := &fakeDownloader{err: context.DeadlineExceeded}
	d := newReceiptDispatcher(&fakeStore{}, &fakeAI{}, send, down)

	d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan",
		router.Message{HasPhoto: true, PhotoFileID: "ph"})

	if want := "Couldn't download that file. Please send it again."; len(send.messages) != 1 || send.messages[0] != want {
		t.Errorf("reply = %q, want %q", send.messages, want)
	}
}

func TestHelpCommand(t *testing.T) {
	send := &fakeSender{}
	fai := &fakeAI{}
	d := newReceiptDispatcher(&fakeStore{}, fai, send, &fakeDownloader{})

	for _, cmd := range []string{"/start", "/help", "/help@financio_bot"} {
		send.messages = nil
		d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan", router.Message{Text: cmd})
		if len(send.messages) != 1 || send.messages[0] != helpText {
			t.Errorf("%s: reply = %q, want helpText", cmd, send.messages)
		}
	}
	if len(fai.requests) != 0 {
		t.Error("help command must not call the AI")
	}
}

// TestPanicRecovery: a panicking handler must not escape Handle; the bot logs
// and keeps answering subsequent messages.
func TestPanicRecovery(t *testing.T) {
	send := &fakeSender{}
	fai := &fakeAI{responses: map[string]ai.ParsedEntry{
		"boom": {IsTransaction: true, Item: "boom", Category: "Meals", Direction: "expense", Amount: 1, Currency: "MYR", Date: "2026-10-18"},
	}}
	fs := &panicStore{fakeStore: &fakeStore{}, onInsert: true}
	d := newReceiptDispatcher(fs.fakeStore, fai, send, &fakeDownloader{})
	d.store = fs

	didPanic := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanic = true
			}
		}()
		d.Handle(context.Background(), 1, 55, 55, "Rifi", "Anggriawan", router.Message{Text: "boom"})
	}()
	if didPanic {
		t.Fatal("panic escaped Handle")
	}

	// The bot keeps answering afterwards.
	fs.onInsert = false
	d.Handle(context.Background(), 2, 55, 55, "Rifi", "Anggriawan", router.Message{Text: "boom"})
	if len(send.messages) == 0 {
		t.Error("bot did not answer after a recovered panic")
	}
}

type panicStore struct {
	*fakeStore
	onInsert bool
}

func (p *panicStore) InsertTransaction(ctx context.Context, txn store.NewTransaction) (store.Transaction, error) {
	if p.onInsert {
		panic("insert exploded")
	}
	return p.fakeStore.InsertTransaction(ctx, txn)
}

// TestPoolOrderingAndConcurrency: same chat runs in order, different chats can
// interleave.
func TestPoolOrderingAndConcurrency(t *testing.T) {
	p := NewPool(4)
	var order []int
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		i := i
		p.Submit(7, func() {
			order = append(order, i)
			if len(order) == 10 {
				close(done)
			}
		})
	}
	<-done
	for i, v := range order {
		if v != i {
			t.Fatalf("order broken: %v", order)
		}
	}
}
