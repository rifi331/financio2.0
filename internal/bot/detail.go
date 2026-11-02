package bot

import (
	"context"
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
	fmt.Fprintf(&b, "🛍️ %s — %s %s", t.Item, t.CurrencyCode, t.TotalAmount)
	if t.Direction == "income" {
		b.WriteString(" (income)")
	}
	fmt.Fprintf(&b, "\n🔠 %s · 🗓️ %s · 🆔 %d\n", t.CategoryName, t.EntryDate.Format("2006-01-02"), t.ID)

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
	details := store.ParseDetails(desc)
	if len(details) == 0 {
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
	type row struct{ item, qty, amount string }
	rows := make([]row, 0, len(details))
	for _, m := range details {
		rows = append(rows, row{gs(m, "item"), gs(m, "qty"), gs(m, "amount")})
	}
	w := [3]int{len("Item"), len("Qty"), len("Amount")}
	for _, r := range rows {
		if len(r.item) > w[0] {
			w[0] = len(r.item)
		}
		if len(r.qty) > w[1] {
			w[1] = len(r.qty)
		}
		if len(r.amount) > w[2] {
			w[2] = len(r.amount)
		}
	}
	pad := func(s string, n int) string {
		for len(s) < n {
			s += " "
		}
		return s
	}
	var b strings.Builder
	b.WriteString(pad("Item", w[0]) + "  " + pad("Qty", w[1]) + "  " + pad("Amount", w[2]) + "\n")
	b.WriteString(strings.Repeat("-", w[0]+w[1]+w[2]+4) + "\n")
	for _, r := range rows {
		b.WriteString(pad(r.item, w[0]) + "  " + pad(r.qty, w[1]) + "  " + pad(r.amount, w[2]) + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// sanitizeMD strips characters that break Telegram Markdown messages.
func sanitizeMD(s string) string {
	s = strings.ReplaceAll(s, "`", "'")
	s = strings.ReplaceAll(s, "```", "'''")
	return s
}
