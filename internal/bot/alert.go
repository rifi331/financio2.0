package bot

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/rifi331/financio/internal/recap"
)

// RunAlerts checks every budget: spend >= 80% (or over) fires a warning once
// per month per budget, sent to that budget group's members. Returns alerts
// sent.
func (d *Dispatcher) RunAlerts(ctx context.Context, now time.Time) int {
	budgets, err := d.store.ListBudgets(ctx, 0) // all groups
	if err != nil {
		d.log.Error("list budgets for alerts failed", "err", err)
		return 0
	}
	month := now.In(d.tz).Format("2006-01")
	sent := 0
	for _, b := range budgets {
		spentStr, err := d.store.MonthCategorySpend(ctx, b.BudgetGroupID, b.CategoryName, b.CurrencyCode, now)
		if err != nil {
			d.log.Warn("alert spend lookup failed", "err", err, "budget", b.CategoryName)
			continue
		}
		spent, _ := strconv.ParseFloat(spentStr, 64)
		limit, _ := strconv.ParseFloat(b.MonthlyLimit, 64)
		if limit <= 0 {
			continue
		}
		pct := spent / limit * 100
		if pct < 80 {
			continue
		}
		need, err := d.store.BudgetNeedsAlert(ctx, b.BudgetGroupID, b.CategoryName, b.CurrencyCode, month)
		if err != nil || !need {
			continue
		}
		dp := 2
		if b.CurrencyCode == "IDR" {
			dp = 0
		}
		text := fmt.Sprintf("Warning: budget alert - %s at %.0f%% of %s %s (%s)",
			b.CategoryName, math.Round(pct), b.CurrencyCode, b.MonthlyLimit,
			recap.FormatAmountCode(b.CurrencyCode, dp, spentStr))
		if spent > limit {
			text += " [OVER]"
		}
		members, err := d.store.GroupMemberIDs(ctx, b.BudgetGroupID)
		if err != nil {
			d.log.Warn("alert members lookup failed", "err", err)
			continue
		}
		delivered, failedCount := 0, 0
		for _, id := range members {
			if err := d.tg.SendMessages(id, []string{text}); err == nil {
				delivered++
			} else {
				failedCount++
			}
		}
		if failedCount > 0 {
			d.log.Warn("budget alert partially delivered", "category", b.CategoryName, "delivered", delivered, "failed", failedCount)
		}
		if delivered > 0 {
			if err := d.store.MarkBudgetAlert(ctx, b.BudgetGroupID, b.CategoryName, b.CurrencyCode, month); err != nil {
				d.log.Warn("mark budget alert failed", "err", err)
			}
			sent++
		}
	}
	return sent
}
