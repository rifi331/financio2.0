package bot

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/rifi331/financio/internal/recap"
	"github.com/rifi331/financio/internal/store"
)

// budgetUsage replies for the /budget command.
const budgetUsage = "Usage:\n/budget list\n/budget set <category> <limit> [currency]\n/budget delete <category> [currency]\nExample: /budget set Meals 500\nBudgets are monthly, per category and currency (default MYR)."

// handleBudget implements the /budget admin command.
func (d *Dispatcher) handleBudget(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	fields := strings.Fields(strings.TrimSpace(text))
	sub := "list"
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}

	switch sub {
	case "list":
		budgets, err := d.store.ListBudgets(ctx)
		if err != nil {
			d.log.Error("list budgets failed", "err", err)
			d.reply(ctx, chatID, "Failed to list budgets.")
			return
		}
		if len(budgets) == 0 {
			d.reply(ctx, chatID, "No budgets set. "+budgetUsage)
			return "budget_list"
		}
		var b strings.Builder
		b.WriteString("Monthly budgets:\n")
		for _, bd := range budgets {
			fmt.Fprintf(&b, "- %s: %s %s/month\n", bd.CategoryName, bd.CurrencyCode, bd.MonthlyLimit)
		}
		d.reply(ctx, chatID, b.String())
		return "budget_list"

	case "set":
		if len(fields) < 4 {
			d.reply(ctx, chatID, budgetUsage)
			return
		}
		cat, limitStr := fields[2], fields[3]
		code := d.defCur
		if len(fields) >= 5 {
			code = strings.ToUpper(fields[4])
		}
		limit, err := strconv.ParseFloat(limitStr, 64)
		if err != nil || limit <= 0 {
			d.reply(ctx, chatID, "Limit must be a positive number, e.g. 500.")
			return
		}
		if !d.knownCategory(ctx, cat) || !d.knownCurrency(ctx, code) {
			d.reply(ctx, chatID, "Unknown category or currency. /budget list shows valid budgets; categories come from the DB.")
			return
		}
		if err := d.store.SetBudget(ctx, cat, code, strconv.FormatFloat(limit, 'f', 2, 64)); err != nil {
			d.log.Error("set budget failed", "err", err)
			d.reply(ctx, chatID, "Failed to save budget.")
			return
		}
		d.reply(ctx, chatID, fmt.Sprintf("OK: %s budget is %s %s/month.", cat, code, limitStr))
		return "budget_set"

	case "delete":
		if len(fields) < 3 {
			d.reply(ctx, chatID, budgetUsage)
			return
		}
		code := d.defCur
		if len(fields) >= 4 {
			code = strings.ToUpper(fields[3])
		}
		deleted, err := d.store.DeleteBudget(ctx, fields[2], code)
		if err != nil {
			d.reply(ctx, chatID, "Failed to delete budget.")
			return
		}
		if deleted {
			d.reply(ctx, chatID, fmt.Sprintf("Budget for %s (%s) removed.", fields[2], code))
		} else {
			d.reply(ctx, chatID, fmt.Sprintf("No budget for %s (%s).", fields[2], code))
		}
		return "budget_delete"
	}

	d.reply(ctx, chatID, budgetUsage)
	return
}

func (d *Dispatcher) knownCategory(ctx context.Context, name string) bool {
	cats, err := d.store.ListActiveCategories(ctx)
	if err != nil {
		return false
	}
	for _, c := range cats {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

func (d *Dispatcher) knownCurrency(ctx context.Context, code string) bool {
	curs, err := d.store.ListActiveCurrencies(ctx)
	if err != nil {
		return false
	}
	for _, c := range curs {
		if strings.EqualFold(c.Code, code) {
			return true
		}
	}
	return false
}

// budgetInfoLines returns "spent/limit (pct%)" lines for every budgeted
// category+currency touched by txns, measured over the calendar month of the
// transaction's own entry_date (household-wide). Empty when no budgets match.
func (d *Dispatcher) budgetInfoLines(ctx context.Context, txns []store.Transaction) []string {
	budgets, err := d.store.ListBudgets(ctx)
	if err != nil || len(budgets) == 0 {
		return nil
	}
	type key struct{ cat, code string }
	limitOf := map[key]string{}
	for _, b := range budgets {
		limitOf[key{b.CategoryName, b.CurrencyCode}] = b.MonthlyLimit
	}

	seen := map[key]bool{}
	var lines []string
	for _, t := range txns {
		k := key{t.CategoryName, t.CurrencyCode}
		if seen[k] {
			continue
		}
		seen[k] = true
		limit, ok := limitOf[k]
		if !ok || t.Direction != "expense" {
			continue
		}
		spent, err := d.store.MonthCategorySpend(ctx, k.cat, k.code, t.EntryDate)
		if err != nil {
			d.log.Warn("budget spend lookup failed", "err", err, "category", k.cat)
			continue
		}
		lines = append(lines, budgetLine(k.cat, k.code, spent, limit))
	}
	return lines
}

// budgetLine renders "📊 Meals: MYR 242.75/MYR 500.00 (49%)" with the
// percentage rounded half up.
func budgetLine(cat, code, spent, limit string) string {
	dp := 2
	if code == "IDR" {
		dp = 0
	}
	s, _ := strconv.ParseFloat(spent, 64)
	l, _ := strconv.ParseFloat(limit, 64)
	pct := 0.0
	if l > 0 {
		pct = math.Round(s / l * 100)
	}
	return fmt.Sprintf("📊 %s: %s/%s (%.0f%%)", cat,
		recap.FormatAmountCode(code, dp, strconv.FormatFloat(s, 'f', -1, 64)),
		recap.FormatAmountCode(code, dp, strconv.FormatFloat(l, 'f', -1, 64)), pct)
}

// withBudgetLines appends budget status lines to a confirmation reply.
func (d *Dispatcher) withBudgetLines(ctx context.Context, reply string, txns []store.Transaction) string {
	lines := d.budgetInfoLines(ctx, txns)
	if len(lines) == 0 {
		return reply
	}
	return reply + "\n" + strings.Join(lines, "\n")
}

// budgetRecapLines computes budget status from the transactions already
// fetched for a recap/report period (sums in memory, not per-month).
func (d *Dispatcher) budgetRecapLines(ctx context.Context, txns []store.Transaction) []string {
	budgets, err := d.store.ListBudgets(ctx)
	if err != nil || len(budgets) == 0 {
		return nil
	}
	limitOf := map[string]map[string]string{}
	for _, b := range budgets {
		if limitOf[b.CategoryName] == nil {
			limitOf[b.CategoryName] = map[string]string{}
		}
		limitOf[b.CategoryName][b.CurrencyCode] = b.MonthlyLimit
	}
	spent := map[string]map[string]float64{}
	for _, t := range txns {
		if t.Direction != "expense" {
			continue
		}
		if spent[t.CategoryName] == nil {
			spent[t.CategoryName] = map[string]float64{}
		}
		s, _ := strconv.ParseFloat(t.TotalAmount, 64)
		spent[t.CategoryName][t.CurrencyCode] += s
	}
	var lines []string
	for cat, codes := range limitOf {
		for code, limit := range codes {
			if s, ok := spent[cat][code]; ok {
				lines = append(lines, budgetLine(cat, code, strconv.FormatFloat(s, 'f', -1, 64), limit))
			}
		}
	}
	return lines
}
