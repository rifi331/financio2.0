package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Budget is a monthly per-category limit, in one currency, owned by a group.
type Budget struct {
	BudgetGroupID int64
	GroupName     string
	CategoryName  string
	CurrencyCode  string
	MonthlyLimit  string // NUMERIC as text
	UpdatedAt     time.Time
}

// ErrUnknownCategory is shared with InsertTransaction.
var errBudgetUnknownCategory = errors.New("unknown category")

// SetBudget upserts the monthly limit for a group's category+currency.
func (s *Store) SetBudget(ctx context.Context, groupID int64, categoryName, currencyCode, limit string) error {
	ct, err := s.pool.Exec(ctx, `
		INSERT INTO budget (budget_group_id, category_id, currency_code, monthly_limit, updated_at)
		SELECT $4, c.id, $2, $3::numeric, now() FROM category c WHERE c.name = $1
		ON CONFLICT (budget_group_id, category_id, currency_code)
		DO UPDATE SET monthly_limit = EXCLUDED.monthly_limit, updated_at = now()`,
		categoryName, currencyCode, limit, groupID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errBudgetUnknownCategory
	}
	return nil
}

// DeleteBudget removes a group's budget; returns false when none existed.
func (s *Store) DeleteBudget(ctx context.Context, groupID int64, categoryName, currencyCode string) (bool, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM budget b USING category c
		WHERE b.category_id = c.id AND c.name = $1 AND b.currency_code = $2
		  AND b.budget_group_id = $3`,
		categoryName, currencyCode, groupID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ListBudgets returns a group's budgets (category names joined), ordered by
// name. groupID <= 0 returns all groups' budgets (alerts runner).
func (s *Store) ListBudgets(ctx context.Context, groupID int64) ([]Budget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT b.budget_group_id, COALESCE(g.name, ''), c.name, b.currency_code, b.monthly_limit::text, b.updated_at
		FROM budget b
		JOIN category c ON c.id = b.category_id
		LEFT JOIN budget_group g ON g.id = b.budget_group_id
		WHERE ($1::bigint <= 0 OR b.budget_group_id = $1)
		ORDER BY c.name, b.currency_code`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Budget
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.BudgetGroupID, &b.GroupName, &b.CategoryName, &b.CurrencyCode, &b.MonthlyLimit, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// MonthCategorySpend sums a group's expenses for one category+currency over
// the calendar month containing t.
func (s *Store) MonthCategorySpend(ctx context.Context, groupID int64, categoryName, currencyCode string, t time.Time) (string, error) {
	var sum string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(t2.total_amount), 0)::text
		FROM transaction_log t2
		JOIN category c ON c.id = t2.category_id
		WHERE c.name = $1 AND t2.currency_code = $2
		  AND t2.direction = 'expense'
		  AND t2.user_id IN (SELECT id FROM app_user WHERE budget_group_id = $4)
		  AND t2.entry_date >= date_trunc('month', $3::date)
		  AND t2.entry_date < date_trunc('month', $3::date) + interval '1 month'`,
		categoryName, currencyCode, t.Format("2006-01-02"), groupID).Scan(&sum)
	if errors.Is(err, pgx.ErrNoRows) {
		return "0", nil
	}
	return sum, err
}

// BudgetAlertState carries the month marker for dedup.
type BudgetAlertState struct {
	BudgetGroupID  int64
	CategoryName   string
	CurrencyCode   string
	LastAlertMonth string
}

// BudgetNeedsAlert reports whether this budget has already alerted in month
// ("2026-10").
func (s *Store) BudgetNeedsAlert(ctx context.Context, groupID int64, catName, code, month string) (bool, error) {
	var m *string
	err := s.pool.QueryRow(ctx, `
		SELECT b.last_alert_month FROM budget b
		JOIN category c ON c.id = b.category_id
		WHERE c.name = $1 AND b.currency_code = $2 AND b.budget_group_id = $3`,
		catName, code, groupID).Scan(&m)
	if err != nil {
		return false, err
	}
	return m == nil || *m != month, nil
}

// MarkBudgetAlert records that this budget alerted in month.
func (s *Store) MarkBudgetAlert(ctx context.Context, groupID int64, catName, code, month string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE budget SET last_alert_month = $4
		WHERE budget_group_id = $3 AND currency_code = $2
		  AND category_id = (SELECT id FROM category WHERE name = $1)`,
		catName, code, groupID, month)
	return err
}
