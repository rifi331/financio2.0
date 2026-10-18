package receipt

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/receipt/testpdf"
	"github.com/rifi331/financio/internal/store"
)

func TestTextOKGuard(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"x", true},
		{strings.Repeat("a", MaxPDFTextBytes), true},
		{strings.Repeat("a", MaxPDFTextBytes+1), false},
	}
	for _, c := range cases {
		if got := TextOK(c.in); got != c.want {
			t.Errorf("TextOK(len %d) = %v, want %v", len(c.in), got, c.want)
		}
	}
}

func TestExtractPDFTextGarbage(t *testing.T) {
	_, err := ExtractPDFText([]byte("not a pdf at all"))
	if !errors.Is(err, ErrUnreadablePDF) {
		t.Errorf("err = %v, want ErrUnreadablePDF", err)
	}
}

func TestUnreadablePDFMessage(t *testing.T) {
	if UnreadablePDFMessage("EN") != "PDF could not be read. Try sending it as a photo instead, or re-export the file." {
		t.Error("EN message mismatch")
	}
	if UnreadablePDFMessage("ID") != "PDF tidak bisa dibaca. Coba kirim ulang sebagai foto, atau ekspor ulang filenya." {
		t.Error("ID message mismatch")
	}
}

func TestFormatMulti(t *testing.T) {
	currencies := []store.Currency{
		{Code: "MYR", Symbol: "RM", DecimalPlaces: 2},
		{Code: "IDR", Symbol: "Rp", DecimalPlaces: 0},
	}
	txns := []store.Transaction{
		{ID: 45, Item: "Giant", CategoryName: "Groceries", CurrencyCode: "MYR", TotalAmount: "16.78", EntryDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)},
		{ID: 46, Item: "Tolls", CategoryName: "Transportation", CurrencyCode: "MYR", TotalAmount: "2.30", EntryDate: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)},
	}
	want := "✅ 2 transactions saved\n" +
		"🆔 45: Giant RM 16.78 (Groceries, 2026-10-14)\n" +
		"🆔 46: Tolls RM 2.30 (Transportation, 2026-10-15)\n" +
		"💰 Total added: MYR 19.08"
	if got := FormatMulti(txns, currencies); got != want {
		t.Errorf("FormatMulti =\n%q\nwant\n%q", got, want)
	}
}

func TestFormatMultiMultiCurrency(t *testing.T) {
	currencies := []store.Currency{
		{Code: "MYR", Symbol: "RM", DecimalPlaces: 2},
		{Code: "IDR", Symbol: "Rp", DecimalPlaces: 0},
	}
	txns := []store.Transaction{
		{ID: 1, Item: "lunch", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "10.00", EntryDate: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Item: "bakso", CategoryName: "Meals", CurrencyCode: "IDR", TotalAmount: "150000", EntryDate: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)},
	}
	got := FormatMulti(txns, currencies)
	want := "✅ 2 transactions saved\n" +
		"🆔 1: lunch RM 10.00 (Meals, 2026-10-14)\n" +
		"🆔 2: bakso Rp 150.000 (Meals, 2026-10-15)\n" +
		"💰 Total added: MYR 10.00 + IDR 150.000"
	if got != want {
		t.Errorf("FormatMulti =\n%q\nwant\n%q", got, want)
	}
}

func TestExtractPDFTextValid(t *testing.T) {
	data := testpdf.Build("Duitnow QR Kopi 12.00 15/10/2026")
	text, err := ExtractPDFText(data)
	if err != nil {
		t.Fatal(err)
	}
	if !TextOK(text) || !strings.Contains(text, "Duitnow QR Kopi 12.00 15/10/2026") {
		t.Errorf("extracted %q", text)
	}
}
