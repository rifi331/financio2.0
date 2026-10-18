// Package store implements the database layer over pgxpool.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User mirrors a row of app_user.
type User struct {
	ID          int64
	DisplayName string
	IsActive    bool
	CreatedAt   time.Time
}

// Currency mirrors a row of currency.
type Currency struct {
	Code          string
	Symbol        string
	DecimalPlaces int16
	SortOrder     int16
}

// Transaction mirrors a row of transaction_log (with joined category name).
type Transaction struct {
	ID           int64
	UserID       int64
	Item         string
	CategoryID   int64
	CategoryName string
	CurrencyCode string
	TotalAmount  string // NUMERIC as exact string to avoid float rounding
	Direction    string
	EntryDate    time.Time
	Source       string
	RawMessage   string
	CreatedAt    time.Time
}

// NewTransaction is the payload for InsertTransaction.
type NewTransaction struct {
	UserID       int64
	Item         string
	Category     string
	CurrencyCode string
	TotalAmount  string
	Direction    string
	EntryDate    time.Time
	Description  []byte // raw JSON for the description JSONB column, may be nil
	Source       string
	RawMessage   string
}

// TxnFilter narrows ListTransactions. UserID is required.
type TxnFilter struct {
	UserID     int64
	From, To   time.Time
	Categories []string
}

// ErrUnknownCategory is returned when InsertTransaction is given a category
// name that does not exist.
var ErrUnknownCategory = errors.New("unknown category")

// Store is the repository over a pgxpool. All methods are context-aware and
// use parameterized SQL exclusively.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a Store.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// EnsureUser upserts the app_user row for telegramID, updating display_name
// when it changed. The user is created lazily on first message, never seeded.
func (s *Store) EnsureUser(ctx context.Context, telegramID int64, displayName string) (User, error) {
	row := s.pool.QueryRow(ctx, `
		INSERT INTO app_user (id, display_name)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE
			SET display_name = EXCLUDED.display_name
			WHERE app_user.display_name IS DISTINCT FROM EXCLUDED.display_name
		RETURNING id, display_name, is_active, created_at`,
		telegramID, displayName)

	var u User
	err := row.Scan(&u.ID, &u.DisplayName, &u.IsActive, &u.CreatedAt)
	if err != nil {
		return User{}, fmt.Errorf("ensure user %d: %w", telegramID, err)
	}
	return u, nil
}

// GetUserByTelegramID returns the app_user row; ok is false when not found.
func (s *Store) GetUserByTelegramID(ctx context.Context, telegramID int64) (User, bool, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id, display_name, is_active, created_at FROM app_user WHERE id = $1`,
		telegramID)

	var u User
	err := row.Scan(&u.ID, &u.DisplayName, &u.IsActive, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, false, nil
	}
	if err != nil {
		return User{}, false, fmt.Errorf("get user %d: %w", telegramID, err)
	}
	return u, true, nil
}

// ListActiveCategories returns active category names in id order.
func (s *Store) ListActiveCategories(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT name FROM category WHERE is_active ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list categories: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, fmt.Errorf("scan category: %w", err)
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// ListActiveCurrencies returns active currencies ordered by sort_order.
func (s *Store) ListActiveCurrencies(ctx context.Context) ([]Currency, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT code, symbol, decimal_places, sort_order
		 FROM currency WHERE is_active ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list currencies: %w", err)
	}
	defer rows.Close()

	var cs []Currency
	for rows.Next() {
		var c Currency
		if err := rows.Scan(&c.Code, &c.Symbol, &c.DecimalPlaces, &c.SortOrder); err != nil {
			return nil, fmt.Errorf("scan currency: %w", err)
		}
		cs = append(cs, c)
	}
	return cs, rows.Err()
}

// InsertTransaction resolves the category by name and inserts in a single
// INSERT .. SELECT .. RETURNING. Returns ErrUnknownCategory for a bad
// category name.
func (s *Store) InsertTransaction(ctx context.Context, txn NewTransaction) (Transaction, error) {
	row := s.pool.QueryRow(ctx, `
		WITH cat AS (
			SELECT id FROM category WHERE name = $2
		)
		INSERT INTO transaction_log
			(user_id, item, category_id, currency_code, total_amount, direction,
			 entry_date, description, source, raw_message)
		SELECT $1, $3, cat.id, $4, $5, $6, $7, $8::jsonb, $9, $10
		FROM cat
		RETURNING id, user_id, item, category_id,
			(SELECT name FROM category WHERE id = category_id),
			currency_code, total_amount::text, direction, entry_date, source,
			COALESCE(raw_message, ''), created_at`,
		txn.UserID, txn.Category, txn.Item, txn.CurrencyCode, txn.TotalAmount,
		txn.Direction, txn.EntryDate, txn.Description, txn.Source, txn.RawMessage)

	t, err := scanTransaction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, fmt.Errorf("%w: %q", ErrUnknownCategory, txn.Category)
	}
	if err != nil {
		return Transaction{}, fmt.Errorf("insert transaction: %w", err)
	}
	return t, nil
}

// DeleteTransactionByID deletes a transaction owned by userID. ok is false
// when no row matched (not found or owned by another user).
func (s *Store) DeleteTransactionByID(ctx context.Context, id, userID int64) (Transaction, bool, error) {
	row := s.pool.QueryRow(ctx, `
		DELETE FROM transaction_log
		WHERE id = $1 AND user_id = $2
		RETURNING id, user_id, item, category_id,
			(SELECT name FROM category WHERE id = category_id),
			currency_code, total_amount::text, direction, entry_date, source,
			COALESCE(raw_message, ''), created_at`,
		id, userID)

	t, err := scanTransaction(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return Transaction{}, false, nil
	}
	if err != nil {
		return Transaction{}, false, fmt.Errorf("delete transaction %d: %w", id, err)
	}
	return t, true, nil
}

// ListTransactions returns transactions matching the filter, ordered by
// entry_date, id.
func (s *Store) ListTransactions(ctx context.Context, f TxnFilter) ([]Transaction, error) {
	clauses := []string{"user_id = $1"}
	args := []any{f.UserID}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if !f.From.IsZero() {
		clauses = append(clauses, "entry_date >= "+arg(f.From))
	}
	if !f.To.IsZero() {
		clauses = append(clauses, "entry_date <= "+arg(f.To))
	}
	if len(f.Categories) > 0 {
		clauses = append(clauses, "category_id = ANY("+arg(f.Categories)+")")
	}

	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.user_id, t.item, t.category_id, c.name,
			t.currency_code, t.total_amount::text, t.direction, t.entry_date,
			t.source, COALESCE(t.raw_message, ''), t.created_at
		FROM transaction_log t
		JOIN category c ON c.id = t.category_id
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY t.entry_date, t.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("list transactions: %w", err)
	}
	defer rows.Close()

	var out []Transaction
	for rows.Next() {
		t, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf("scan transaction: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func scanTransaction(row pgx.Row) (Transaction, error) {
	var t Transaction
	err := row.Scan(&t.ID, &t.UserID, &t.Item, &t.CategoryID, &t.CategoryName,
		&t.CurrencyCode, &t.TotalAmount, &t.Direction, &t.EntryDate, &t.Source,
		&t.RawMessage, &t.CreatedAt)
	return t, err
}

// DashboardFilter narrows ListDashboardTransactions. Zero fields are unused.
type DashboardFilter struct {
	From, To  time.Time // entry_date range
	Category  string    // exact category name, "" = all
	Direction string    // expense | income, "" = all
}

// DashboardTxn is a Transaction with the owner's display name, for the web UI.
type DashboardTxn struct {
	Transaction
	UserName string
}

// ListDashboardTransactions returns transactions across all users (the web
// dashboard is household-wide), newest first, at most limit rows.
func (s *Store) ListDashboardTransactions(ctx context.Context, f DashboardFilter, limit int) ([]DashboardTxn, error) {
	clauses := []string{"TRUE"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if !f.From.IsZero() {
		clauses = append(clauses, "t.entry_date >= "+arg(f.From))
	}
	if !f.To.IsZero() {
		clauses = append(clauses, "t.entry_date < "+arg(f.To))
	}
	if f.Category != "" {
		clauses = append(clauses, "c.name = "+arg(f.Category))
	}
	if f.Direction != "" {
		clauses = append(clauses, "t.direction = "+arg(f.Direction))
	}
	if limit <= 0 {
		limit = 200
	}
	clauses = append(clauses, "TRUE")

	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.user_id, t.item, t.category_id, c.name,
			t.currency_code, t.total_amount::text, t.direction, t.entry_date,
			t.source, COALESCE(t.raw_message, ''), t.created_at,
			COALESCE(u.display_name, '')
		FROM transaction_log t
		JOIN category c ON c.id = t.category_id
		JOIN app_user u ON u.id = t.user_id
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY t.entry_date DESC, t.id DESC
		LIMIT `+arg(limit), args...)
	if err != nil {
		return nil, fmt.Errorf("list dashboard transactions: %w", err)
	}
	defer rows.Close()

	var out []DashboardTxn
	for rows.Next() {
		var d DashboardTxn
		if err := rows.Scan(&d.ID, &d.UserID, &d.Item, &d.CategoryID, &d.CategoryName,
			&d.CurrencyCode, &d.TotalAmount, &d.Direction, &d.EntryDate, &d.Source,
			&d.RawMessage, &d.CreatedAt, &d.UserName); err != nil {
			return nil, fmt.Errorf("scan dashboard transaction: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
