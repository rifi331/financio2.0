// Package receipt holds the media-receipt helpers shared by the photo and PDF
// flows: PDF text extraction, the 30KB text guard, and the compact
// multi-transaction confirmation format.
package receipt

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	pdf "github.com/ledongthuc/pdf"

	"github.com/rifi331/financio/internal/recap"
	"github.com/rifi331/financio/internal/store"
)

// MaxPDFTextBytes is the extracted-text ceiling: larger statements are not
// sent to the model.
const MaxPDFTextBytes = 30 * 1024

// ErrUnreadablePDF is returned when text extraction yields nothing usable.
var ErrUnreadablePDF = errors.New("pdf could not be read")

// ExtractPDFText pulls plain text out of a statement PDF via ledongthuc/pdf.
func ExtractPDFText(data []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrUnreadablePDF, err)
	}
	var b strings.Builder
	for i := 1; i <= r.NumPage(); i++ {
		text, err := r.Page(i).GetPlainText(nil)
		if err != nil {
			continue // tolerate one bad page
		}
		b.WriteString(text)
	}
	return b.String(), nil
}

// TextOK reports whether extracted text is usable: non-empty and within the
// size guard.
func TextOK(text string) bool {
	return len(text) > 0 && len(text) <= MaxPDFTextBytes
}

// UnreadablePDFMessage is the localized reply for an unreadable PDF.
func UnreadablePDFMessage(lang string) string {
	if lang == "ID" {
		return "PDF tidak bisa dibaca. Coba kirim ulang sebagai foto, atau ekspor ulang filenya."
	}
	return "PDF could not be read. Try sending it as a photo instead, or re-export the file."
}

// FormatMulti renders the compact confirmation used when >1 transaction is
// inserted from one photo/PDF:
//
//	✅ 3 transactions saved
//	🆔 45: Giant RM 16.78 (Groceries, 2026-10-14)
//	💰 Total added: MYR 19.08
func FormatMulti(txns []store.Transaction, currencies []store.Currency) string {
	var b strings.Builder
	fmt.Fprintf(&b, "✅ %d transactions saved\n", len(txns))
	for _, t := range txns {
		cur := byCode(currencies, t.CurrencyCode)
		fmt.Fprintf(&b, "🆔 %d: %s %s (%s, %s)\n",
			t.ID, t.Item, recap.FormatAmount(cur, t.TotalAmount), t.CategoryName, t.EntryDate.Format("2006-01-02"))
	}
	fmt.Fprintf(&b, "💰 Total added: %s", totals(txns, currencies))
	return b.String()
}

// totals sums per currency: "MYR 19.08" or "MYR 19.08 + IDR 150.000".
func totals(txns []store.Transaction, currencies []store.Currency) string {
	var order []string
	sums := map[string]float64{}
	for _, t := range txns {
		v, err := strconv.ParseFloat(t.TotalAmount, 64)
		if err != nil {
			continue
		}
		if _, ok := sums[t.CurrencyCode]; !ok {
			order = append(order, t.CurrencyCode)
		}
		sums[t.CurrencyCode] += v
	}
	var parts []string
	for _, code := range order {
		cur := byCode(currencies, code)
		parts = append(parts, recap.FormatAmountCode(code, int(cur.DecimalPlaces),
			strconv.FormatFloat(sums[code], 'f', int(cur.DecimalPlaces), 64)))
	}
	return strings.Join(parts, " + ")
}

func byCode(cs []store.Currency, code string) *store.Currency {
	for i := range cs {
		if cs[i].Code == code {
			return &cs[i]
		}
	}
	return &store.Currency{Code: code, DecimalPlaces: 2}
}
