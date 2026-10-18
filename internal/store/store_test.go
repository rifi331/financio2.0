package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rifi331/financio/migrations"
)

// newTestStore skips when FINANCIO_TEST_DSN is unset, otherwise migrates and
// returns a Store. Test data is cleaned up per test.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("FINANCIO_TEST_DSN")
	if dsn == "" {
		t.Skip("FINANCIO_TEST_DSN not set; skipping database tests")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)

	// Migrations must be idempotent: run twice.
	for i := 0; i < 2; i++ {
		if err := Migrate(ctx, pool, migrations.FS); err != nil {
			t.Fatalf("migrate (run %d): %v", i+1, err)
		}
	}

	pool.Exec(ctx, `DELETE FROM transaction_log`)
	s := New(pool)
	t.Cleanup(func() {
		pool.Exec(context.Background(),
			`DELETE FROM transaction_log; DELETE FROM app_user`)
	})
	return s
}

func TestSeedsPresent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	cats, err := s.ListActiveCategories(ctx)
	if err != nil {
		t.Fatalf("ListActiveCategories: %v", err)
	}
	if len(cats) != 10 {
		t.Errorf("expected 10 categories, got %d: %v", len(cats), cats)
	}

	curs, err := s.ListActiveCurrencies(ctx)
	if err != nil {
		t.Fatalf("ListActiveCurrencies: %v", err)
	}
	want := []string{"MYR", "IDR", "USD", "SGD"}
	if len(curs) != 4 {
		t.Fatalf("expected 4 currencies, got %d: %v", len(curs), curs)
	}
	for i, c := range curs {
		if c.Code != want[i] {
			t.Errorf("currency[%d] = %s, want %s", i, c.Code, want[i])
		}
	}
	if curs[1].DecimalPlaces != 0 {
		t.Errorf("IDR decimal_places = %d, want 0", curs[1].DecimalPlaces)
	}
}

func TestEnsureUserUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	const tid = int64(990001)

	u, err := s.EnsureUser(ctx, tid, "Alice Test")
	if err != nil {
		t.Fatalf("EnsureUser create: %v", err)
	}
	if u.DisplayName != "Alice Test" {
		t.Errorf("display_name = %q, want Alice Test", u.DisplayName)
	}

	u2, err := s.EnsureUser(ctx, tid, "Alice Renamed")
	if err != nil {
		t.Fatalf("EnsureUser update: %v", err)
	}
	if u2.DisplayName != "Alice Renamed" {
		t.Errorf("display_name after upsert = %q, want Alice Renamed", u2.DisplayName)
	}

	got, ok, err := s.GetUserByTelegramID(ctx, tid)
	if err != nil || !ok {
		t.Fatalf("GetUserByTelegramID: ok=%v err=%v", ok, err)
	}
	if got.DisplayName != "Alice Renamed" {
		t.Errorf("stored display_name = %q, want Alice Renamed", got.DisplayName)
	}

	if _, ok, err := s.GetUserByTelegramID(ctx, 990002); err != nil || ok {
		t.Errorf("unknown user: ok=%v err=%v, want ok=false", ok, err)
	}
}

func mustUser(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	u, err := s.EnsureUser(context.Background(), 991000, name)
	if err != nil {
		t.Fatalf("EnsureUser: %v", err)
	}
	return u.ID
}

func TestInsertUnknownCategoryFails(t *testing.T) {
	s := newTestStore(t)
	uid := mustUser(t, s, "Cat Tester")

	_, err := s.InsertTransaction(context.Background(), NewTransaction{
		UserID: uid, Item: "x", Category: "Nope", CurrencyCode: "MYR",
		TotalAmount: "10.00", Direction: "expense",
		EntryDate: time.Now(), Source: "text",
	})
	if !errors.Is(err, ErrUnknownCategory) {
		t.Fatalf("err = %v, want ErrUnknownCategory", err)
	}
}

func TestDeleteOwnership(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	owner := mustUser(t, s, "Owner")
	other := func() int64 {
		u, err := s.EnsureUser(ctx, 992000, "Other")
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}()

	txn, err := s.InsertTransaction(ctx, NewTransaction{
		UserID: owner, Item: "nasi lemak", Category: "Meals", CurrencyCode: "MYR",
		TotalAmount: "12.50", Direction: "expense",
		EntryDate: time.Now().Truncate(24 * time.Hour), Source: "text",
		RawMessage: "lunch rm12.50",
	})
	if err != nil {
		t.Fatalf("InsertTransaction: %v", err)
	}
	if txn.CategoryName != "Meals" || txn.TotalAmount != "12.50" {
		t.Errorf("stored row wrong: %+v", txn)
	}

	// Other user's delete must not touch it.
	if _, ok, err := s.DeleteTransactionByID(ctx, txn.ID, other); err != nil || ok {
		t.Fatalf("other's delete: ok=%v err=%v, want ok=false", ok, err)
	}
	// Owner's delete works and returns the row.
	del, ok, err := s.DeleteTransactionByID(ctx, txn.ID, owner)
	if err != nil || !ok {
		t.Fatalf("owner delete: ok=%v err=%v", ok, err)
	}
	if del.ID != txn.ID || del.Item != "nasi lemak" {
		t.Errorf("deleted row = %+v", del)
	}
	// Second delete: not found.
	if _, ok, err := s.DeleteTransactionByID(ctx, txn.ID, owner); err != nil || ok {
		t.Errorf("re-delete: ok=%v err=%v, want ok=false", ok, err)
	}
}

func TestListFilters(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	uid := mustUser(t, s, "List Tester")
	day := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)

	seed := []NewTransaction{
		{UserID: uid, Item: "a", Category: "Meals", CurrencyCode: "MYR", TotalAmount: "1.00", EntryDate: day, Source: "text"},
		{UserID: uid, Item: "b", Category: "Groceries", CurrencyCode: "MYR", TotalAmount: "2.00", EntryDate: day.AddDate(0, 0, 1), Source: "text"},
		{UserID: uid, Item: "c", Category: "Meals", CurrencyCode: "MYR", TotalAmount: "3.00", EntryDate: day.AddDate(0, 0, 2), Source: "text"},
		{UserID: uid, Item: "d", Category: "Medical", CurrencyCode: "MYR", TotalAmount: "4.00", EntryDate: day.AddDate(0, 0, 2), Source: "text"},
	}
	for i, n := range seed {
		n.Direction = "expense"
		if _, err := s.InsertTransaction(ctx, n); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}

	items := func(txns []Transaction) string {
		out := ""
		for _, t := range txns {
			out += t.Item
		}
		return out
	}

	txns, err := s.ListTransactions(ctx, TxnFilter{UserID: uid})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if items(txns) != "abcd" {
		t.Errorf("all = %q, want abcd", items(txns))
	}

	txns, err = s.ListTransactions(ctx, TxnFilter{UserID: uid, From: day.AddDate(0, 0, 1), To: day.AddDate(0, 0, 1)})
	if err != nil {
		t.Fatalf("list date range: %v", err)
	}
	if items(txns) != "b" {
		t.Errorf("date range = %q, want b", items(txns))
	}

	txns, err = s.ListTransactions(ctx, TxnFilter{UserID: uid, Categories: []string{"Meals"}})
	if err != nil {
		t.Fatalf("list category: %v", err)
	}
	if items(txns) != "ac" {
		t.Errorf("category filter = %q, want ac", items(txns))
	}

	txns, err = s.ListTransactions(ctx, TxnFilter{
		UserID: uid, From: day.AddDate(0, 0, 2), To: day.AddDate(0, 0, 2),
		Categories: []string{"Meals", "Medical"},
	})
	if err != nil {
		t.Fatalf("list both: %v", err)
	}
	if items(txns) != "cd" {
		t.Errorf("both filters = %q, want cd", items(txns))
	}
}
