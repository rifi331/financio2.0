package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Budget is a monthly per-category limit, in one currency.
type Budget struct {
	CategoryName string
	CurrencyCode string
	MonthlyLimit string // NUMERIC as text
	UpdatedAt    time.Time
}

// ErrUnknownCategory is shared with InsertTransaction.
var errBudgetUnknownCategory = errors.New("unknown category")

// SetBudget upserts the monthly limit for a category+currency (household-wide).
func (s *Store) SetBudget(ctx context.Context, categoryName, currencyCode, limit string) error {
	ct, err := s.pool.Exec(ctx, `
		INSERT INTO budget (category_id, currency_code, monthly_limit, updated_at)
		SELECT c.id, $2, $3::numeric, now() FROM category c WHERE c.name = $1
		ON CONFLICT (category_id, currency_code)
		DO UPDATE SET monthly_limit = EXCLUDED.monthly_limit, updated_at = now()`,
		categoryName, currencyCode, limit)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return errBudgetUnknownCategory
	}
	return nil
}

// DeleteBudget removes a budget; returns false when none existed.
func (s *Store) DeleteBudget(ctx context.Context, categoryName, currencyCode string) (bool, error) {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM budget b USING category c
		WHERE b.category_id = c.id AND c.name = $1 AND b.currency_code = $2`,
		categoryName, currencyCode)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ListBudgets returns all budgets (category names joined), ordered by name.
func (s *Store) ListBudgets(ctx context.Context) ([]Budget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.name, b.currency_code, b.monthly_limit::text, b.updated_at
		FROM budget b JOIN category c ON c.id = b.category_id
		ORDER BY c.name, b.currency_code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Budget
	for rows.Next() {
		var b Budget
		if err := rows.Scan(&b.CategoryName, &b.CurrencyCode, &b.MonthlyLimit, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// MonthCategorySpend sums expenses for one category+currency over the
// calendar month containing t (household-wide, all users).
func (s *Store) MonthCategorySpend(ctx context.Context, categoryName, currencyCode string, t time.Time) (string, error) {
	var sum string
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(t2.total_amount), 0)::text
		FROM transaction_log t2
		JOIN category c ON c.id = t2.category_id
		WHERE c.name = $1 AND t2.currency_code = $2
		  AND t2.direction = 'expense'
		  AND t2.entry_date >= date_trunc('month', $3::date)
		  AND t2.entry_date < date_trunc('month', $3::date) + interval '1 month'`,
		categoryName, currencyCode, t.Format("2006-01-02")).Scan(&sum)
	if errors.Is(err, pgx.ErrNoRows) {
		return "0", nil
	}
	return sum, err
}
