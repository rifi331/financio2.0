// Package report builds the full-report PDF: summary, expense pie chart per
// currency, and income/expense transaction tables.
package report

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/rifi331/financio/internal/store"
)

var palette = [][3]uint8{
	{31, 119, 180}, {255, 127, 14}, {44, 160, 44}, {214, 39, 40},
	{148, 103, 189}, {140, 86, 75}, {227, 119, 194}, {127, 127, 127},
	{188, 189, 34}, {23, 190, 207}, {174, 90, 60}, {90, 110, 174},
	{210, 160, 30}, {60, 160, 110}, {160, 60, 120}, {100, 140, 60},
}

// Data is everything Build needs.
type Data struct {
	From, To   time.Time
	Txns       []store.Transaction // both directions
	Currencies []store.Currency    // for decimal places
	TZ         *time.Location
	Generated  time.Time
}

func decimalPlaces(currencies []store.Currency, code string) int {
	for _, c := range currencies {
		if c.Code == code {
			return int(c.DecimalPlaces)
		}
	}
	if code == "IDR" {
		return 0
	}
	return 2
}

func formatAmount(code string, dp int, amount float64) string {
	if dp == 0 {
		return fmt.Sprintf("%s %.0f", code, amount)
	}
	return fmt.Sprintf("%s %.*f", code, dp, amount)
}

// Build renders the report PDF and returns its bytes.
func Build(d Data) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(fmt.Sprintf("Financio report %s", d.From.Format("Jan 2006")), false)
	pdf.SetAuthor("Financio", false)
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 18)

	expenses, income := split(d.Txns)

	// --- Section 1: summary ---
	secSummary := pdf.AddLink()
	pdf.AddPage()
	pageSummary := pdf.PageNo()
	title(pdf, fmt.Sprintf("Financio — Full Report %s", d.From.In(d.TZ).Format("January 2006")))
	subtitle(pdf, fmt.Sprintf("Period: %s to %s · Generated %s · %d transactions",
		d.From.In(d.TZ).Format("2 Jan 2006"), d.To.In(d.TZ).AddDate(0, 0, -1).Format("2 Jan 2006"),
		d.Generated.In(d.TZ).Format("2 Jan 2006 15:04"), len(d.Txns)))

	totals := map[string]float64{} // code -> sum, income positive, expense negative
	for _, t := range expenses {
		amt, _ := parse(t.TotalAmount)
		totals[t.CurrencyCode] -= amt
	}
	for _, t := range income {
		amt, _ := parse(t.TotalAmount)
		totals[t.CurrencyCode] += amt
	}
	incBy := sumBy(income)
	expBy := sumBy(expenses)

	pdf.Ln(4)
	heading(pdf, "Summary")
	for _, code := range sortedCodes(totals) {
		dp := decimalPlaces(d.Currencies, code)
		row(pdf, 90, []string{"Income (" + code + ")", formatAmount(code, dp, incBy[code])})
		row(pdf, 90, []string{"Expenses (" + code + ")", formatAmount(code, dp, expBy[code])})
		row(pdf, 90, []string{"Net (" + code + ")", formatAmount(code, dp, incBy[code]-expBy[code])})
		pdf.Ln(2)
	}
	note(pdf, "Use the PDF bookmarks/sidebar to jump between sections.")

	// --- Section 2: expense pie per currency ---
	secPie := pdf.AddLink()
	pdf.AddPage()
	pagePie := pdf.PageNo()
	title(pdf, "Expenses by Category")
	byCur := groupByCurrency(expenses)
	codes := make([]string, 0, len(byCur))
	for c := range byCur {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	for _, code := range codes {
		catSums := sumByCategory(byCur[code])
		if len(catSums) == 0 {
			continue
		}
		pdf.Ln(2)
		heading(pdf, code)
		drawPie(pdf, catSums, code, decimalPlaces(d.Currencies, code))
	}

	// --- Section 3: expense transactions ---
	secExp := pdf.AddLink()
	pdf.AddPage()
	pageExp := pdf.PageNo()
	title(pdf, "Expenses")
	txnTable(pdf, expenses, d)

	// --- Section 4: income transactions ---
	secInc := pdf.AddLink()
	pdf.AddPage()
	pageInc := pdf.PageNo()
	title(pdf, "Income")
	txnTable(pdf, income, d)

	// Outline (bookmarks) for reader navigation.
	pdf.SetLink(secSummary, 0, pageSummary)
	pdf.SetLink(secPie, 0, pagePie)
	pdf.SetLink(secExp, 0, pageExp)
	pdf.SetLink(secInc, 0, pageInc)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func split(txns []store.Transaction) (expenses, income []store.Transaction) {
	for _, t := range txns {
		if t.Direction == "income" {
			income = append(income, t)
		} else {
			expenses = append(expenses, t)
		}
	}
	return
}

func parse(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%g", &f)
	return f, err
}

func sumBy(txns []store.Transaction) map[string]float64 {
	m := map[string]float64{}
	for _, t := range txns {
		amt, _ := parse(t.TotalAmount)
		m[t.CurrencyCode] += amt
	}
	return m
}

func sumByCategory(txns []store.Transaction) map[string]float64 {
	m := map[string]float64{}
	for _, t := range txns {
		amt, _ := parse(t.TotalAmount)
		m[t.CategoryName] += amt
	}
	return m
}

func groupByCurrency(txns []store.Transaction) map[string][]store.Transaction {
	m := map[string][]store.Transaction{}
	for _, t := range txns {
		m[t.CurrencyCode] = append(m[t.CurrencyCode], t)
	}
	return m
}

func sortedCodes(m map[string]float64) []string {
	codes := make([]string, 0, len(m))
	for c := range m {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	return codes
}

// drawPie renders a pie of category sums with a legend of percentages.
func drawPie(pdf *fpdf.Fpdf, catSums map[string]float64, code string, dp int) {
	names := make([]string, 0, len(catSums))
	total := 0.0
	for n, v := range catSums {
		names = append(names, n)
		total += v
	}
	sort.Slice(names, func(i, j int) bool { return catSums[names[i]] > catSums[names[j]] })

	cx, cy, r := 55.0, 62.0, 25.0
	start := 0.0
	pdf.SetFont("Helvetica", "", 9)
	for i, n := range names {
		frac := catSums[n] / total
		end := start + frac*360
		col := palette[i%len(palette)]
		pdf.SetFillColor(int(col[0]), int(col[1]), int(col[2]))
		// Wedge as a filled polygon: center, arc approximated by segments.
		pdf.SetLineWidth(0.1)
		pdf.SetDrawColor(255, 255, 255)
		path := []fpdf.PointType{{X: cx, Y: cy}}
		steps := int(math.Max(2, (end-start)/4))
		for s := 0; s <= steps; s++ {
			a := (start + (end-start)*float64(s)/float64(steps)) * math.Pi / 180
			path = append(path, fpdf.PointType{X: cx + r*math.Cos(a), Y: cy + r*math.Sin(a)})
		}
		pdf.MoveTo(path[0].X, path[0].Y)
		for _, p := range path[1:] {
			pdf.LineTo(p.X, p.Y)
		}
		pdf.ClosePath()
		pdf.DrawPath("F")
		start = end

		// Legend entry beside the pie.
		ly := 30.0 + float64(i)*6.5
		pdf.SetXY(105, ly)
		pdf.CellFormat(5, 5, "", "0", 0, "C", true, 0, "")
		pdf.SetXY(112, ly)
		pdf.CellFormat(0, 5, fmt.Sprintf("%s — %s (%.1f%%)", n,
			formatAmount(code, dp, catSums[n]), frac*100), "0", 0, "L", false, 0, "")
	}
	pdf.SetXY(15, cy+r+8)
	pdf.CellFormat(0, 6, fmt.Sprintf("Total %s", formatAmount(code, dp, total)), "0", 1, "L", false, 0, "")
	pdf.Ln(6)
}

func txnTable(pdf *fpdf.Fpdf, txns []store.Transaction, d Data) {
	if len(txns) == 0 {
		note(pdf, "No transactions.")
		return
	}
	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(230, 234, 238)
	w := []float64{22, 95, 40, 28}
	for i, h := range []string{"Date", "Item", "Category", "Amount"} {
		pdf.CellFormat(w[i], 6, h, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Helvetica", "", 8)
	if pdf.GetY() > 250 {
		pdf.AddPage()
	}
	for _, t := range txns {
		if pdf.GetY() > 270 {
			pdf.AddPage()
			pdf.SetFont("Helvetica", "", 8)
		}
		amt, _ := parse(t.TotalAmount)
		dp := decimalPlaces(d.Currencies, t.CurrencyCode)
		for i, cell := range []string{
			t.EntryDate.In(d.TZ).Format("02 Jan 06"),
			truncate(t.Item, 55),
			t.CategoryName,
			formatAmount(t.CurrencyCode, dp, amt),
		} {
			pdf.CellFormat(w[i], 5.5, cell, "1", 0, "L", false, 0, "")
		}
		pdf.Ln(-1)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func title(pdf *fpdf.Fpdf, s string) {
	pdf.SetFont("Helvetica", "B", 16)
	pdf.SetTextColor(28, 39, 51)
	pdf.CellFormat(0, 10, s, "", 1, "L", false, 0, "")
}

func subtitle(pdf *fpdf.Fpdf, s string) {
	pdf.SetFont("Helvetica", "", 9)
	pdf.SetTextColor(90, 107, 123)
	pdf.CellFormat(0, 6, s, "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func heading(pdf *fpdf.Fpdf, s string) {
	pdf.SetFont("Helvetica", "B", 12)
	pdf.SetTextColor(28, 39, 51)
	pdf.CellFormat(0, 8, s, "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}

func row(pdf *fpdf.Fpdf, labelW float64, cols []string) {
	pdf.SetFont("Helvetica", "", 10)
	pdf.CellFormat(labelW, 6, cols[0], "", 0, "L", false, 0, "")
	pdf.SetFont("Helvetica", "B", 10)
	pdf.CellFormat(0, 6, cols[1], "", 1, "R", false, 0, "")
}

func note(pdf *fpdf.Fpdf, s string) {
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(120, 120, 120)
	pdf.CellFormat(0, 5, s, "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
}
