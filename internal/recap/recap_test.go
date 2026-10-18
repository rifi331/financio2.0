package recap

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/store"
)

var testCurrencies = []store.Currency{
	{Code: "MYR", Symbol: "RM", DecimalPlaces: 2, SortOrder: 1},
	{Code: "IDR", Symbol: "Rp", DecimalPlaces: 0, SortOrder: 2},
	{Code: "USD", Symbol: "$", DecimalPlaces: 2, SortOrder: 3},
}

var from = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
var to = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

func d(day int) time.Time { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }

func TestFormatGroupingAndMultiCurrencyTotals(t *testing.T) {
	txns := []store.Transaction{
		// deliberately out of order — store orders by (entry_date, id);
		// formatter groups consecutive same-date rows and prints in given order.
		{ID: 2, UserID: 1, Item: "tol", CategoryName: "Public Transportation", CurrencyCode: "IDR", TotalAmount: "150000", EntryDate: d(17)},
		{ID: 1, UserID: 1, Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "10.00", EntryDate: d(17)},
		{ID: 4, UserID: 1, Item: "coffee", CategoryName: "Meals", CurrencyCode: "USD", TotalAmount: "4.50", EntryDate: d(18)},
		{ID: 3, UserID: 1, Item: "ayam", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "20.00", EntryDate: d(18)},
	}

	got := Format(txns, from, to, "EN", testCurrencies, false)

	want := "📅 Summary 2026-10-01 - 2026-10-31\n" +
		"- 🗓️ 2026-10-17:\n" +
		"  - 🆔: 2, 🛍️: tol, 💵: Rp 150.000\n" +
		"  - 🆔: 1, 🛍️: nasi lemak, 💵: RM 10.00\n" +
		"- 🗓️ 2026-10-18:\n" +
		"  - 🆔: 4, 🛍️: coffee, 💵: $ 4.50\n" +
		"  - 🆔: 3, 🛍️: ayam, 💵: RM 20.00\n" +
		"💰 Total: MYR 30.00 + IDR 150.000 + USD 4.50\n" +
		"🔠 by category: Meals MYR 30.00 + USD 4.50 | Public Transportation IDR 150.000"
	if len(got) != 1 || got[0] != want {
		t.Errorf("got =\n%q\nwant\n%q", strings.Join(got, "\x00"), want)
	}
}

func TestFormatSingleCategoryNoCategoryLine(t *testing.T) {
	txns := []store.Transaction{
		{ID: 1, UserID: 1, Item: "a", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "5.00", EntryDate: d(2)},
	}
	got := Format(txns, from, to, "EN", testCurrencies, false)
	if strings.Contains(got[0], "by category") {
		t.Errorf("category line should be omitted for single category:\n%s", got[0])
	}
}

func TestFormatIndonesian(t *testing.T) {
	txns := []store.Transaction{
		{ID: 1, UserID: 1, Item: "bakso", CategoryName: "Meals", CurrencyCode: "IDR", TotalAmount: "50000", EntryDate: d(5)},
	}
	got := Format(txns, from, to, "ID", testCurrencies, false)
	want := "📅 Ringkasan 2026-10-01 - 2026-10-31\n" +
		"- 🗓️ 2026-10-05:\n" +
		"  - 🆔: 1, 🛍️: bakso, 💵: Rp 50.000\n" +
		"💰 Total: IDR 50.000"
	if got[0] != want {
		t.Errorf("got =\n%q\nwant\n%q", got[0], want)
	}
}

func TestFormatEmpty(t *testing.T) {
	got := Format(nil, from, to, "EN", testCurrencies, false)
	// Empty input normally short-circuits to the empty-period message in the
	// dispatcher; the formatter still returns a parseable header-only chunk.
	if len(got) == 0 || !strings.HasPrefix(got[0], "📅 Summary") {
		t.Errorf("got = %q", got)
	}
}

func TestFormatSplitsAt4000WithHeaders(t *testing.T) {
	var txns []store.Transaction
	for i := 1; i <= 120; i++ {
		day := 1 + i/30
		txns = append(txns, store.Transaction{
			ID: int64(i), UserID: 1,
			Item:         "very long item name number " + strings.Repeat("x", 80) + " " + itoa(i),
			CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "10.00",
			EntryDate: d(day),
		})
	}

	got := Format(txns, from, to, "EN", testCurrencies, false)

	if len(got) < 2 {
		t.Fatalf("expected split into multiple messages, got %d", len(got))
	}
	for i, chunk := range got {
		if len(chunk) > 4000 {
			t.Errorf("chunk %d len = %d > 4000", i, len(chunk))
		}
		if !strings.HasPrefix(chunk, "📅 Summary 2026-10-01 - 2026-10-31") {
			t.Errorf("chunk %d missing header: %.60q", i, chunk)
		}
	}
	last := got[len(got)-1]
	if !strings.Contains(last, "💰 Total:") {
		t.Error("total block must be in the last message")
	}
	// No line may be cut: every item line ends with its closing content and
	// every chunk's item lines start with the two-space bullet.
	for i, chunk := range got {
		for _, ln := range strings.Split(chunk, "\n")[1:] {
			if strings.HasPrefix(ln, "  - 🆔:") {
				if !strings.Contains(ln, "💵: RM 10.00") {
					t.Errorf("chunk %d has a cut item line: %q", i, ln)
				}
			}
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestNoSQLConcatenation(t *testing.T) {
	// The formatter deals only in typed rows; nothing user-controlled ever
	// becomes SQL. Guard the invariant that the recap path holds no query
	// building at all: the package must not import a database driver.
	files, err := readPackageFiles()
	if err != nil {
		t.Skip(err)
	}
	for _, f := range files {
		if strings.Contains(f, "jackc/pgx") {
			t.Error("recap package must not touch the database driver directly")
		}
		if strings.Contains(f, "Sprintf") && strings.Contains(strings.ToLower(f), "select") {
			t.Error("suspicious SQL string building in recap package")
		}
	}
}

func readPackageFiles() ([]string, error) {
	ents, err := os.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			b, err := os.ReadFile(e.Name())
			if err != nil {
				return nil, err
			}
			out = append(out, string(b))
		}
	}
	return out, nil
}
