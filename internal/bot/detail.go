package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rifi331/financio/internal/store"
)

// handleDetail replies with one transaction's full detail, line items as a
// monospace table (Telegram renders ``` blocks fixed-width).
func (d *Dispatcher) handleDetail(ctx context.Context, chatID int64, user store.User, id int64) (outcome string) {
	outcome = "error"
	t, ok, err := d.store.GetTransactionDetail(ctx, id, user.ID)
	if err != nil {
		d.log.Error("detail lookup failed", "err", err, "id", id)
		d.reply(ctx, chatID, recapFailHint("EN"))
		return
	}
	if !ok {
		d.reply(ctx, chatID, "Record not found!")
		return "not_found"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "🆔: %d\n🛍️: %s\n🔠: %s\n💵: %s %s\n", t.ID, t.Item, t.CategoryName, t.CurrencyCode, t.TotalAmount)
	if t.Direction == "income" {
		b.WriteString("↩️: Income\n")
	}
	fmt.Fprintf(&b, "🗓️: %s\n👤: %s\n📎: %s\n🕒: %s\n",
		t.EntryDate.Format("2006-01-02"), t.UserName, t.Source, t.CreatedAt.In(d.tz).Format("2006-01-02 15:04"))

	if table := detailTable(t.Description); table != "" {
		b.WriteString("\n```\n" + table + "\n```\n")
	}
	if t.RawMessage != "" {
		rm := t.RawMessage
		if len(rm) > 300 {
			rm = rm[:300] + "..."
		}
		b.WriteString("\n💬: " + sanitizeMD(rm))
	}

	if err := d.tg.SendMarkdown(chatID, b.String()); err != nil {
		// Markdown-sensitive content can still fail; fall back to plain text.
		d.reply(ctx, chatID, strings.ReplaceAll(b.String(), "```", ""))
		return "detail_plain"
	}
	return "detail"
}

// detailTable renders {"details":[...]} as a fixed-width table; empty string
// when there are no line items.
func detailTable(desc []byte) string {
	if len(desc) == 0 {
		return ""
	}
	var doc struct {
		Details []map[string]any `json:"details"`
	}
	if err := json.Unmarshal(desc, &doc); err != nil || len(doc.Details) == 0 {
		return ""
	}
	gs := func(m map[string]any, k string) string {
		if v, ok := m[k]; ok && v != nil {
			if f, ok := v.(float64); ok {
				return fmt.Sprintf("%g", f)
			}
			return sanitizeMD(fmt.Sprintf("%v", v))
		}
		return ""
	}
	type row struct{ item, amount, qty, location, note string }
	rows := make([]row, 0, len(doc.Details))
	for _, m := range doc.Details {
		rows = append(rows, row{gs(m, "item"), gs(m, "amount"), gs(m, "qty"), gs(m, "location"), gs(m, "note")})
	}
	w := [5]int{len("Item"), len("Amount"), len("Qty"), len("Location"), len("Note")}
	for _, r := range rows {
		if len(r.item) > w[0] {
			w[0] = len(r.item)
		}
		if len(r.amount) > w[1] {
			w[1] = len(r.amount)
		}
		if len(r.qty) > w[2] {
			w[2] = len(r.qty)
		}
		if len(r.location) > w[3] {
			w[3] = len(r.location)
		}
		if len(r.note) > w[4] {
			w[4] = len(r.note)
		}
	}
	pad := func(s string, n int) string {
		for len(s) < n {
			s += " "
		}
		return s
	}
	var b strings.Builder
	b.WriteString(pad("Item", w[0]) + "  " + pad("Amount", w[1]) + "  " + pad("Qty", w[2]) + "  " + pad("Location", w[3]) + "  " + "Note\n")
	b.WriteString(strings.Repeat("-", w[0]+w[1]+w[2]+w[3]+8) + "\n")
	for _, r := range rows {
		b.WriteString(pad(r.item, w[0]) + "  " + pad(r.amount, w[1]) + "  " + pad(r.qty, w[2]) + "  " + pad(r.location, w[3]) + "  " + r.note + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// sanitizeMD strips characters that break Telegram Markdown messages.
func sanitizeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "```", "'''")
	return s
}
