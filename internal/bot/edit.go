package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/store"
)

const editUsage = "Usage: edit <id> [amount 12.50] [category Meals] [item new label] [date 2026-10-01] [currency MYR]"

var editKeys = map[string]bool{"amount": true, "category": true, "item": true, "date": true, "currency": true}

// handleEdit applies partial updates to one transaction: edit 123 amount
// 12.50 category Snacks. Values validated before the store call; not-found
// and not-owned share the generic reply (same rule as delete).
func (d *Dispatcher) handleEdit(ctx context.Context, chatID int64, user store.User, id int64, spec string) (outcome string) {
	outcome = "error"
	patch, err := parseEditSpec(spec)
	if err != nil {
		d.reply(ctx, chatID, err.Error())
		outcome = "invalid"
		return
	}
	if err := d.validateEdit(ctx, patch); err != nil {
		d.reply(ctx, chatID, err.Error()+"\n"+editUsage)
		outcome = "invalid"
		return
	}
	txn, ok, err := d.store.UpdateTransactionFields(ctx, id, user.ID, *patch)
	if err != nil {
		d.log.Error("edit failed", "err", err, "id", id)
		d.reply(ctx, chatID, recapFailHint(d.langOf(spec)))
		return
	}
	if !ok {
		d.reply(ctx, chatID, "Record not found!")
		outcome = "not_found"
		return
	}
	currencies, err := d.store.ListActiveCurrencies(ctx)
	if err != nil {
		currencies = nil
	}
	reply := d.withBudgetLines(ctx, user, formatConfirmation(txn, currencies, firstName(user.DisplayName), "EN"), []store.Transaction{txn})
	d.reply(ctx, chatID, "✏️ updated:\n"+reply)
	return "edited"
}

// parseEditSpec splits "amount 12.50 category Snacks" into key/value pairs.
// Item takes the rest of the line until the next known key.
func parseEditSpec(spec string) (*store.TxnPatch, error) {
	fields := strings.Fields(spec)
	var patch store.TxnPatch
	set := func(key, val string) error {
		switch key {
		case "amount":
			f, err := strconv.ParseFloat(val, 64)
			if err != nil || f <= 0 {
				return fmt.Errorf("amount must be a positive number")
			}
			patch.Amount = ptr(strconv.FormatFloat(f, 'f', 2, 64))
		case "category":
			patch.Category = ptr(val)
		case "item":
			patch.Item = ptr(val)
		case "date":
			if _, err := time.Parse("2006-01-02", val); err != nil {
				return fmt.Errorf("date must look like 2026-10-01")
			}
			patch.Date = ptr(val)
		case "currency":
			patch.CurrencyCode = ptr(strings.ToUpper(val))
		}
		return nil
	}
	for i := 0; i < len(fields); i++ {
		key := strings.ToLower(fields[i])
		if !editKeys[key] {
			return nil, fmt.Errorf("unknown field %q\n%s", fields[i], editUsage)
		}
		if i+1 >= len(fields) {
			return nil, fmt.Errorf("missing value for %s\n%s", key, editUsage)
		}
		if key == "item" {
			var parts []string
			i++
			for i < len(fields) && !editKeys[strings.ToLower(fields[i])] {
				parts = append(parts, fields[i])
				i++
			}
			i--
			if len(parts) == 0 {
				return nil, fmt.Errorf("missing value for item\n%s", editUsage)
			}
			patch.Item = ptr(strings.Join(parts, " "))
			continue
		}
		if err := set(key, fields[i+1]); err != nil {
			return nil, err
		}
		i++
	}
	return &patch, nil
}

// validateEdit checks category/currency against the DB lists.
func (d *Dispatcher) validateEdit(ctx context.Context, p *store.TxnPatch) error {
	if p.Category != nil && !d.knownCategory(ctx, *p.Category) {
		return fmt.Errorf("unknown category %q", *p.Category)
	}
	if p.CurrencyCode != nil && !d.knownCurrency(ctx, *p.CurrencyCode) {
		return fmt.Errorf("unknown currency %q", *p.CurrencyCode)
	}
	return nil
}

func (d *Dispatcher) langOf(text string) string {
	if d.tz == nil {
		return "EN"
	}
	_ = text
	return "EN"
}

func ptr(s string) *string { return &s }
