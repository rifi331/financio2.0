// Package recap renders transaction lists into Telegram recap messages.
package recap

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/store"
)

const maxLen = 4000 // Telegram hard limit is 4096; stay safe

// FormatAmount renders a NUMERIC amount string per the currency's rules:
// IDR (0 decimals) -> "Rp 150.000" with dot thousands separators; others use
// comma thousands and dot decimals ("RM 10.50", "S$ 1,234.50").
func FormatAmount(c *store.Currency, amount string) string {
	if c == nil {
		return amount
	}
	grouped := groupDigits(intPart(amount), int(c.DecimalPlaces))
	if c.DecimalPlaces == 0 {
		return c.Symbol + " " + grouped
	}
	return c.Symbol + " " + grouped + "." + fracPart(amount, int(c.DecimalPlaces))
}

// FormatAmountCode is FormatAmount with the ISO code as prefix, for totals
// lines ("MYR 123.40 + IDR 250.000").
func FormatAmountCode(code string, decimalPlaces int, amount string) string {
	grouped := groupDigits(intPart(amount), decimalPlaces)
	if decimalPlaces == 0 {
		return code + " " + grouped
	}
	return code + " " + grouped + "." + fracPart(amount, decimalPlaces)
}

func intPart(amount string) string {
	if i := strings.IndexByte(amount, '.'); i >= 0 {
		amount = amount[:i]
	}
	whole := strings.TrimLeft(amount, "0")
	if whole == "" {
		return "0"
	}
	return whole
}

func fracPart(amount string, dp int) string {
	frac := ""
	if i := strings.IndexByte(amount, '.'); i >= 0 {
		frac = amount[i+1:]
	}
	if len(frac) < dp {
		frac += strings.Repeat("0", dp-len(frac))
	}
	return frac[:dp]
}

func groupDigits(whole string, dp int) string {
	sep := ","
	if dp == 0 {
		sep = "."
	}
	var parts []string
	for len(whole) > 3 {
		parts = append([]string{whole[len(whole)-3:]}, parts...)
		whole = whole[:len(whole)-3]
	}
	parts = append([]string{whole}, parts...)
	return strings.Join(parts, sep)
}

// Format builds one or more recap messages (each <= 4000 chars) from the
// transactions. orderBy "category" groups them under category headings
// (rows by date within each); anything else is the default date grouping.
// The total block is always in the last message; a continuation re-emits
// the summary header.
func Format(txns []store.Transaction, from, to time.Time, lang string, currencies []store.Currency, filterHadCategories bool, orderBy string) []string {
	header := summaryHeader(from, to, lang)

	var lines []string
	if orderBy == "category" {
		var catsSeen []string
		catSet := map[string]bool{}
		for _, t := range txns {
			if !catSet[t.CategoryName] {
				catSet[t.CategoryName] = true
				catsSeen = append(catsSeen, t.CategoryName)
			}
		}
		for _, c := range catsSeen {
			lines = append(lines, "- 🔠 "+c+":")
			for _, t := range txns {
				if t.CategoryName != c {
					continue
				}
				cur := findByCode(currencies, t.CurrencyCode)
				lines = append(lines, fmt.Sprintf("  - 🗓️ %s, 🆔: %d, 🛍️: %s, 💵: %s",
					t.EntryDate.Format("2006-01-02"), t.ID, t.Item, FormatAmount(cur, t.TotalAmount)))
			}
		}
	} else {
		var lastDate string
		for _, t := range txns {
			d := t.EntryDate.Format("2006-01-02")
			if d != lastDate {
				lines = append(lines, "- 🗓️ "+d+":")
				lastDate = d
			}
			cur := findByCode(currencies, t.CurrencyCode)
			lines = append(lines, fmt.Sprintf("  - 🆔: %d, 🛍️: %s, 💵: %s",
				t.ID, t.Item, FormatAmount(cur, t.TotalAmount)))
		}
	}

	footer := totalLines(txns, currencies, lang, filterHadCategories)

	// Assemble chunks: each starts with the header; never cut a line;
	// footer goes at the end of the last chunk (or its own chunk).
	var chunks []string
	current := header
	flush := func() {
		chunks = append(chunks, strings.TrimRight(current, "\n"))
		current = header
	}
	for _, ln := range lines {
		if len(current)+len(ln)+2 > maxLen && current != header {
			flush()
		}
		current += "\n" + ln
	}
	for _, ln := range footer {
		if len(current)+len(ln)+2 > maxLen && current != header {
			flush()
		}
		current += "\n" + ln
	}
	if len(chunks) == 0 || current != header {
		flush()
	}
	return chunks
}

func summaryHeader(from, to time.Time, lang string) string {
	if lang == "ID" {
		return fmt.Sprintf("📅 Ringkasan %s - %s", from.Format("2006-01-02"), to.Format("2006-01-02"))
	}
	return fmt.Sprintf("📅 Summary %s - %s", from.Format("2006-01-02"), to.Format("2006-01-02"))
}

// totalLines renders the per-currency total and, conditionally, the
// per-category totals.
func totalLines(txns []store.Transaction, currencies []store.Currency, lang string, filterHadCategories bool) []string {
	totalLabel, catLabel := "💰 Total:", "🔠 by category:"
	if lang == "ID" {
		totalLabel, catLabel = "💰 Total:", "🔠 per kategori:"
	}

	// totals per currency code, in first-seen order of the currency table
	totals := map[string]float64{}
	var order []string
	for _, c := range currencies {
		order = append(order, c.Code)
	}
	perCat := map[string]map[string]float64{} // category -> code -> sum
	var catsSeen []string
	catSet := map[string]bool{}

	for _, t := range txns {
		amt, _ := strconv.ParseFloat(t.TotalAmount, 64)
		if _, ok := totals[t.CurrencyCode]; !ok && !contains(order, t.CurrencyCode) {
			order = append(order, t.CurrencyCode)
		}
		totals[t.CurrencyCode] += amt
		if perCat[t.CategoryName] == nil {
			perCat[t.CategoryName] = map[string]float64{}
		}
		perCat[t.CategoryName][t.CurrencyCode] += amt
		if !catSet[t.CategoryName] {
			catSet[t.CategoryName] = true
			catsSeen = append(catsSeen, t.CategoryName)
		}
	}

	var parts []string
	for _, code := range order {
		if totals[code] > 0 {
			parts = append(parts, formatSum(code, decimalPlacesOf(currencies, code), totals[code]))
		}
	}
	if len(parts) == 0 {
		return nil
	}

	lines := []string{totalLabel + " " + strings.Join(parts, " + ")}

	if filterHadCategories || len(catsSeen) > 1 {
		sort.Strings(catsSeen)
		var catParts []string
		for _, cat := range catsSeen {
			var curParts []string
			for _, code := range order {
				if perCat[cat][code] > 0 {
					curParts = append(curParts, formatSum(code, decimalPlacesOf(currencies, code), perCat[cat][code]))
				}
			}
			if len(curParts) > 0 {
				catParts = append(catParts, cat+" "+strings.Join(curParts, " + "))
			}
		}
		lines = append(lines, catLabel+" "+strings.Join(catParts, " | "))
	}
	return lines
}

func formatSum(code string, dp int, sum float64) string {
	return FormatAmountCode(code, dp, strconv.FormatFloat(sum, 'f', dp, 64))
}

func decimalPlacesOf(currencies []store.Currency, code string) int {
	for _, c := range currencies {
		if c.Code == code {
			return int(c.DecimalPlaces)
		}
	}
	return 2
}

func findByCode(currencies []store.Currency, code string) *store.Currency {
	for i := range currencies {
		if currencies[i].Code == code {
			return &currencies[i]
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
