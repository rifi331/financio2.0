package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/store"
)

const recurringUsage = "Usage:\n/recurring add <item> <amount> [day N] [category X] [income]\n/recurring list\n/recurring delete <id>\nExample: /recurring add Rent 1200 day 1 category Housing"

// handleRecurring implements the /recurring command.
func (d *Dispatcher) handleRecurring(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	fields := strings.Fields(strings.TrimSpace(text))
	sub := "list"
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}

	switch sub {
	case "add":
		if len(fields) < 4 {
			d.reply(ctx, chatID, recurringUsage)
			return
		}
		args := fields[2:]
		itemWords := []string{}
		i := 0
		day, category, direction := 1, "Others", "expense"
		for i < len(args) {
			k := strings.ToLower(args[i])
			if k == "income" {
				direction = "income"
				i++
				continue
			}
			if (k == "day" || k == "category") && i+1 < len(args) {
				if k == "day" {
					n, err := strconv.Atoi(args[i+1])
					if err != nil || n < 1 || n > 31 {
						d.reply(ctx, chatID, "day must be 1-31")
						return
					}
					day = n
				} else {
					category = args[i+1]
				}
				i += 2
				continue
			}
			itemWords = append(itemWords, args[i])
			i++
		}
		if len(itemWords) == 0 {
			d.reply(ctx, chatID, recurringUsage)
			return
		}
		// The amount is the last bare number in the item words.
		amount, err := strconv.ParseFloat(itemWords[len(itemWords)-1], 64)
		if err != nil || amount <= 0 {
			d.reply(ctx, chatID, "Could not find a positive amount.\n"+recurringUsage)
			return
		}
		itemWords = itemWords[:len(itemWords)-1]
		if len(itemWords) == 0 {
			d.reply(ctx, chatID, recurringUsage)
			return
		}
		item := strings.Join(itemWords, " ")
		if !d.knownCategory(ctx, category) {
			d.reply(ctx, chatID, fmt.Sprintf("Unknown category %q.", category))
			return
		}
		r, err := d.store.AddRecurring(ctx, store.Recurring{
			UserID: user.ID, Item: item, CategoryName: category,
			CurrencyCode: d.defCur, Amount: strconv.FormatFloat(amount, 'f', 2, 64),
			Direction: direction, DayOfMonth: day,
		})
		if err != nil {
			d.log.Error("add recurring failed", "err", err)
			d.reply(ctx, chatID, "Failed to save recurring rule.")
			return
		}
		d.reply(ctx, chatID, fmt.Sprintf("OK: #%d %s %s %s/month on day %d (%s).",
			r.ID, r.CurrencyCode, r.Amount, r.Item, r.DayOfMonth, r.Direction))
		return "recurring_add"

	case "list":
		rules, err := d.store.ListRecurring(ctx, user.ID)
		if err != nil {
			d.reply(ctx, chatID, "Failed to list recurring rules.")
			return
		}
		if len(rules) == 0 {
			d.reply(ctx, chatID, "No recurring rules. "+recurringUsage)
			return "recurring_list"
		}
		var b strings.Builder
		b.WriteString("Recurring rules:\n")
		for _, r := range rules {
			fmt.Fprintf(&b, "- #%d %s %s %s, day %d (%s)\n", r.ID, r.CurrencyCode, r.Amount, r.Item, r.DayOfMonth, r.Direction)
		}
		d.reply(ctx, chatID, b.String())
		return "recurring_list"

	case "delete":
		if len(fields) < 3 {
			d.reply(ctx, chatID, recurringUsage)
			return
		}
		id, _ := strconv.ParseInt(fields[2], 10, 64)
		deleted, err := d.store.DeleteRecurring(ctx, id, user.ID)
		if err != nil {
			d.reply(ctx, chatID, "Failed to delete recurring rule.")
			return
		}
		if deleted {
			d.reply(ctx, chatID, fmt.Sprintf("Recurring #%d removed.", id))
		} else {
			d.reply(ctx, chatID, "Record not found!")
		}
		return "recurring_delete"
	}

	d.reply(ctx, chatID, recurringUsage)
	return
}

// RunDue inserts every due recurring rule for today's month and confirms to
// each owner's private chat (chat id == telegram id). Idempotent per month.
// Returns the number of transactions inserted.
func (d *Dispatcher) RunDue(ctx context.Context, now time.Time) int {
	due, err := d.store.DueRecurring(ctx, now)
	if err != nil {
		d.log.Error("due recurring failed", "err", err)
		return 0
	}
	if len(due) == 0 {
		return 0
	}
	categories, _ := d.store.ListActiveCategories(ctx)
	currencies, _ := d.store.ListActiveCurrencies(ctx)
	inserted := 0
	now = now.In(d.tz)
	for _, r := range due {
		user, ok, err := d.auth.Authorize(ctx, r.UserID, "", "")
		if err != nil || !ok || user.ID == 0 {
			d.log.Error("recurring owner lookup failed", "err", err, "user_id", r.UserID)
			continue
		}
		day := clampDay(r.DayOfMonth, now)
		if day > now.Day() {
			continue // the rule's (clamped) day hasn't arrived yet this month
		}
		entryDate := time.Date(now.Year(), now.Month(), day, 0, 0, 0, 0, d.tz)
		txn, err := d.insertParsed(ctx, user, ai.ParsedEntry{
			IsTransaction: true, Item: r.Item, Category: r.CategoryName,
			Direction: r.Direction, Amount: mustFloat(r.Amount), Currency: r.CurrencyCode,
			Date: entryDate.Format("2006-01-02"),
		}, categories, currencies, now, "recurring", "recurring #"+fmt.Sprint(r.ID))
		if err != nil {
			d.log.Error("recurring insert failed", "err", err, "recurring_id", r.ID)
			continue
		}
		if err := d.store.MarkRecurringRun(ctx, r.ID, now); err != nil {
			d.log.Error("mark recurring run failed", "err", err, "recurring_id", r.ID)
		}
		reply := d.withBudgetLines(ctx, formatConfirmation(txn, currencies, firstName(user.DisplayName), "EN"), []store.Transaction{txn})
		_ = d.tg.SendMessages(r.UserID, []string{"recurring:\n" + reply})
		inserted++
	}
	return inserted
}

// clampDay keeps day within the month (31 -> 28/29/30 as needed).
func clampDay(day int, t time.Time) int {
	if day < 1 {
		return 1
	}
	last := time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, t.Location()).Day()
	if day > last {
		return last
	}
	return day
}

func mustFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
