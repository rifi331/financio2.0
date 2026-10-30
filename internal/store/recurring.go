package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Recurring is a monthly auto-transaction owned by one user.
type Recurring struct {
	ID           int64
	UserID       int64
	Item         string
	CategoryName string
	CurrencyCode string
	Amount       string // NUMERIC as text
	Direction    string
	DayOfMonth   int
	LastRun      *time.Time
	IsActive     bool
}

// ErrUnknownCategory is shared with InsertTransaction.
var errRecurringUnknownCategory = errors.New("unknown category")

// AddRecurring validates the category exists and inserts the rule.
func (s *Store) AddRecurring(ctx context.Context, r Recurring) (Recurring, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO recurring (user_id, item, category_name, currency_code, amount, direction, day_of_month)
		SELECT $1, $2, $3, $4, $5::numeric, $6, $7
		WHERE EXISTS (SELECT 1 FROM category WHERE name = $3)
		RETURNING id, user_id, item, category_name, currency_code, amount::text, direction, day_of_month, last_run, is_active`,
		r.UserID, r.Item, r.CategoryName, r.CurrencyCode, r.Amount, r.Direction, r.DayOfMonth)
	out, err := scanRecurring(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Recurring{}, errRecurringUnknownCategory
	}
	return out, err
}

// ListRecurring returns a user's rules, newest first.
func (s *Store) ListRecurring(ctx context.Context, userID int64) ([]Recurring, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, item, category_name, currency_code, amount::text, direction, day_of_month, last_run, is_active
		FROM recurring WHERE user_id = $1 ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recurring
	for rows.Next() {
		r, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteRecurring removes a rule scoped to its owner.
func (s *Store) DeleteRecurring(ctx context.Context, id, userID int64) (bool, error) {
	ct, err := s.pool.Exec(ctx, `DELETE FROM recurring WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// DueRecurring returns active rules due in today's calendar month: day_of_month
// <= the month's last day (clamped) and last_run outside this month.
func (s *Store) DueRecurring(ctx context.Context, today time.Time) ([]Recurring, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, item, category_name, currency_code, amount::text, direction, day_of_month, last_run, is_active
		FROM recurring
		WHERE is_active
		  AND day_of_month <= EXTRACT(DAY FROM date_trunc('month', $1::date) + interval '1 month - 1 day')::int
		  AND COALESCE(date_trunc('month', last_run), date_trunc('month', '1900-01-01'::date)) <> date_trunc('month', $1::date)`,
		today.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recurring
	for rows.Next() {
		r, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// MarkRecurringRun stamps last_run so a rule runs once per month.
func (s *Store) MarkRecurringRun(ctx context.Context, id int64, day time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE recurring SET last_run = $2 WHERE id = $1`, id, day.Format("2006-01-02"))
	return err
}

func scanRecurring(row pgx.Row) (Recurring, error) {
	var r Recurring
	err := row.Scan(&r.ID, &r.UserID, &r.Item, &r.CategoryName, &r.CurrencyCode,
		&r.Amount, &r.Direction, &r.DayOfMonth, &r.LastRun, &r.IsActive)
	return r, err
}

// HasRecurringEntry reports whether the runner already inserted a
// transaction for this rule in the calendar month of t (idempotency guard:
// insert and MarkRecurringRun are not in one transaction, so a crash between
// them must not double-post on the next tick).
func (s *Store) HasRecurringEntry(ctx context.Context, ruleID int64, t time.Time) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx, `
		SELECT count(*) FROM transaction_log
		WHERE source = 'recurring' AND raw_message = $1
		  AND entry_date >= date_trunc('month', $2::date)
		  AND entry_date < date_trunc('month', $2::date) + interval '1 month'`,
		"recurring #"+fmt.Sprint(ruleID), t.Format("2006-01-02")).Scan(&n)
	return n > 0, err
}
