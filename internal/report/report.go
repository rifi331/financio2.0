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
	// GroupByCategory renders the tables grouped under category headings
	// (rows by date within each) instead of one date-ordered table.
	GroupByCategory bool
	// Budgets are the monthly per-category limits to report against.
	Budgets []store.Budget
	// PrevTxns is the previous calendar month's transactions for the
	// month-over-month comparison block.
	PrevTxns []store.Transaction
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
	title(pdf, fmt.Sprintf("Financio - Full Report %s", d.From.In(d.TZ).Format("January 2006")))
	subtitle(pdf, fmt.Sprintf("Period: %s to %s - Generated %s - %d transactions",
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
	if n := countTransfers(d.Txns); n > 0 {
		note(pdf, fmt.Sprintf("%d transfer(s) between users are excluded from totals.", n))
	}

	drawMonthOverMonth(pdf, d, expenses, income)

	// --- Per-currency sections: charts page + that currency's tables.
	// drawPie renders at fixed page coordinates, so every currency gets its
	// own fresh page — multiple currencies on one page drew wedges on top of
	// each other. Tables are split by currency too so MYR rows never mix
	// with USD/IDR ones.
	byCur := groupByCurrency(expenses)
	codes := make([]string, 0, len(byCur))
	for c := range byCur {
		codes = append(codes, c)
	}
	incCur := groupByCurrency(income)
	for c := range incCur {
		if _, ok := byCur[c]; !ok {
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)

	secCharts := pdf.AddLink()
	pageCharts := 0
	secTables := pdf.AddLink()
	pageTables := 0
	for _, code := range codes {
		catSums := sumByCategory(byCur[code])

		// Charts page for this currency (own page: pie uses fixed coords).
		pdf.AddPage()
		if pageCharts == 0 {
			pageCharts = pdf.PageNo()
		}
		title(pdf, "Expenses by Category ("+code+")")
		if len(catSums) > 0 {
			drawPie(pdf, catSums, code, decimalPlaces(d.Currencies, code))
			drawBars(pdf, catSums, code, decimalPlaces(d.Currencies, code))
			drawInsights(pdf, catSums, code, decimalPlaces(d.Currencies, code))
		} else {
			note(pdf, "No expenses in this currency.")
		}
		drawBudgets(pdf, d, expenses, code, decimalPlaces(d.Currencies, code))

		// Tables for this currency.
		expRows := filterCurrency(expenses, code)
		incRows := filterCurrency(income, code)
		if len(expRows) == 0 && len(incRows) == 0 {
			continue
		}
		pdf.AddPage()
		if pageTables == 0 {
			pageTables = pdf.PageNo()
		}
		title(pdf, "Transactions ("+code+")")
		heading(pdf, "Expenses "+code)
		txnTable(pdf, expRows, d)
		if len(incRows) > 0 {
			pdf.Ln(4)
			heading(pdf, "Income "+code)
			txnTable(pdf, incRows, d)
		}
	}

	// Outline (bookmarks) for reader navigation.
	pdf.SetLink(secSummary, 0, pageSummary)
	if pageCharts > 0 {
		pdf.SetLink(secCharts, 0, pageCharts)
	}
	if pageTables > 0 {
		pdf.SetLink(secTables, 0, pageTables)
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func split(txns []store.Transaction) (expenses, income []store.Transaction) {
	for _, t := range txns {
		switch t.Direction {
		case "income":
			income = append(income, t)
		case "transfer":
			// account movement between registered users: neither side of
			// the report's money math
		default:
			expenses = append(expenses, t)
		}
	}
	return
}

// countTransfers reports how many rows are account movements.
func countTransfers(txns []store.Transaction) int {
	n := 0
	for _, t := range txns {
		if t.Direction == "transfer" {
			n++
		}
	}
	return n
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
		legend := fmt.Sprintf("%s - %s (%.1f%%)", n,
			formatAmount(code, dp, catSums[n]), frac*100)
		pdf.CellFormat(0, 5, fitText(pdf, legend, 195-112), "0", 0, "L", false, 0, "")
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
	if d.GroupByCategory {
		byCat := map[string][]store.Transaction{}
		var cats []string
		for _, t := range txns {
			if _, ok := byCat[t.CategoryName]; !ok {
				cats = append(cats, t.CategoryName)
			}
			byCat[t.CategoryName] = append(byCat[t.CategoryName], t)
		}
		sort.Strings(cats)
		for _, c := range cats {
			if pdf.GetY() > 250 {
				pdf.AddPage()
			}
			heading(pdf, c)
			txnRows(pdf, byCat[c], d)
			// Per-category subtotal, per currency present in this group.
			sub := sumBy(byCat[c])
			subParts := make([]string, 0, len(sub))
			for _, code := range sortedCodes(sub) {
				subParts = append(subParts, formatAmount(code, decimalPlaces(d.Currencies, code), sub[code]))
			}
			pdf.SetFont("Helvetica", "B", 9)
			pdf.SetTextColor(90, 107, 123)
			pdf.CellFormat(0, 6, fitText(pdf, "Subtotal: "+strings.Join(subParts, " + "), 165), "", 1, "R", false, 0, "")
			pdf.SetTextColor(0, 0, 0)
			pdf.Ln(2)
		}
		return
	}
	txnRows(pdf, txns, d)
}

func txnRows(pdf *fpdf.Fpdf, txns []store.Transaction, d Data) {
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
			fitText(pdf, t.Item, w[1]-2),
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
	return s[:n-1] + "..."
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

// sortedCatNames returns category names ordered by spend, largest first.
func sortedCatNames(catSums map[string]float64) []string {
	names := make([]string, 0, len(catSums))
	total := 0.0
	for n, v := range catSums {
		names = append(names, n)
		total += v
	}
	sort.Slice(names, func(i, j int) bool { return catSums[names[i]] > catSums[names[j]] })
	return names
}

// drawBars renders a horizontal bar chart of category sums below the pie.
func drawBars(pdf *fpdf.Fpdf, catSums map[string]float64, code string, dp int) {
	names := sortedCatNames(catSums)
	if len(names) == 0 {
		return
	}
	max := catSums[names[0]]
	if max <= 0 {
		return
	}

	pdf.Ln(2)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(28, 39, 51)
	pdf.CellFormat(0, 6, "Bar chart", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	const labelW, barMaxW, rowH = 55.0, 90.0, 6.0
	x0 := pdf.GetX()
	for i, n := range names {
		y := pdf.GetY()
		pdf.SetXY(x0, y)
		pdf.SetFont("Helvetica", "", 8)
		pdf.CellFormat(labelW, rowH, fitText(pdf, n, labelW-2), "", 0, "L", false, 0, "")
		w := barMaxW * (catSums[n] / max)
		col := palette[i%len(palette)]
		pdf.SetFillColor(int(col[0]), int(col[1]), int(col[2]))
		pdf.Rect(x0+labelW, y+1, w, rowH-2, "F")
		pdf.SetXY(x0+labelW+barMaxW+2, y)
		pdf.CellFormat(0, rowH, formatAmount(code, dp, catSums[n]), "", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
}

// drawInsights lists the top 3 categories by spend with share of total.
func drawInsights(pdf *fpdf.Fpdf, catSums map[string]float64, code string, dp int) {
	names := sortedCatNames(catSums)
	total := 0.0
	for _, v := range catSums {
		total += v
	}
	if total <= 0 || len(names) == 0 {
		return
	}

	pdf.Ln(1)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(28, 39, 51)
	pdf.CellFormat(0, 6, "Insights - top 3 expenses ("+code+")", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 9)
	top := 3
	if len(names) < top {
		top = len(names)
	}
	for i := 0; i < top; i++ {
		n := names[i]
		pdf.CellFormat(0, 5.5, fmt.Sprintf("%d. %s - %s (%.1f%% of %s expenses)",
			i+1, n, formatAmount(code, dp, catSums[n]), catSums[n]/total*100, code),
			"", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
}

// drawBudgets lists each budget for this currency with the period's spend
// against the monthly limit.
func drawBudgets(pdf *fpdf.Fpdf, d Data, expenses []store.Transaction, code string, dp int) {
	spentByCat := sumByCategory(nil)
	for _, t := range expenses {
		if t.CurrencyCode != code || t.Direction != "expense" {
			continue
		}
		amt, _ := parse(t.TotalAmount)
		spentByCat[t.CategoryName] += amt
	}
	var rows [][2]string // category, limit
	for _, b := range d.Budgets {
		if b.CurrencyCode != code {
			continue
		}
		rows = append(rows, [2]string{b.CategoryName, b.MonthlyLimit})
	}
	if len(rows) == 0 {
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })

	pdf.Ln(1)
	pdf.SetFont("Helvetica", "B", 10)
	pdf.SetTextColor(28, 39, 51)
	pdf.CellFormat(0, 6, "Budgets ("+code+")", "", 1, "L", false, 0, "")
	pdf.SetTextColor(0, 0, 0)
	pdf.SetFont("Helvetica", "", 9)
	for _, r := range rows {
		limit, _ := parse(r[1])
		spent := spentByCat[r[0]]
		pct := 0.0
		if limit > 0 {
			pct = math.Round(spent / limit * 100)
		}
		status := ""
		if spent > limit {
			status = "  [OVER]"
		}
		pdf.CellFormat(0, 5.5, fmt.Sprintf("%s: %s/%s (%.0f%%)%s",
			r[0], formatAmount(code, dp, spent), formatAmount(code, dp, limit), pct, status),
			"", 1, "L", false, 0, "")
	}
	pdf.Ln(2)
}

// drawMonthOverMonth renders income/expense/net per currency for this month
// vs the previous one, with signed delta percent ("new" when prev is 0).
func drawMonthOverMonth(pdf *fpdf.Fpdf, d Data, expenses, income []store.Transaction) {
	if len(d.PrevTxns) == 0 {
		return
	}
	prevExp, prevInc := split(d.PrevTxns)

	cur := map[string][2]float64{} // code -> {income, expense}
	for _, t := range income {
		a, _ := parse(t.TotalAmount)
		c := cur[t.CurrencyCode]
		cur[t.CurrencyCode] = [2]float64{c[0] + a, c[1]}
	}
	for _, t := range expenses {
		a, _ := parse(t.TotalAmount)
		c := cur[t.CurrencyCode]
		cur[t.CurrencyCode] = [2]float64{c[0], c[1] + a}
	}
	prev := map[string][2]float64{}
	for _, t := range prevInc {
		a, _ := parse(t.TotalAmount)
		c := prev[t.CurrencyCode]
		prev[t.CurrencyCode] = [2]float64{c[0] + a, c[1]}
	}
	for _, t := range prevExp {
		a, _ := parse(t.TotalAmount)
		c := prev[t.CurrencyCode]
		prev[t.CurrencyCode] = [2]float64{c[0], c[1] + a}
	}

	pdf.Ln(4)
	heading(pdf, "vs previous month")
	pdf.SetFont("Helvetica", "", 9)
	pdf.CellFormat(30, 5.5, "code", "1", 0, "L", true, 0, "")
	pdf.CellFormat(0, 5.5, "income / prev   expenses / prev   net / prev", "1", 1, "L", true, 0, "")
	codes := make([]string, 0, len(cur))
	for c := range cur {
		codes = append(codes, c)
	}
	for c := range prev {
		if _, ok := cur[c]; !ok {
			codes = append(codes, c)
		}
	}
	sort.Strings(codes)
	for _, code := range codes {
		dp := decimalPlaces(d.Currencies, code)
		ci, ce := cur[code][0], cur[code][1]
		pi, pe := prev[code][0], prev[code][1]
		pdf.CellFormat(30, 5.5, code, "1", 0, "L", false, 0, "")
		pdf.CellFormat(0, 5.5, fitText(pdf, fmt.Sprintf("%s / %s   %s / %s   %s / %s",
			formatAmount(code, dp, ci), formatAmount(code, dp, pi),
			formatAmount(code, dp, ce), formatAmount(code, dp, pe),
			formatAmount(code, dp, ci-ce), formatAmount(code, dp, pi-pe)), 150),
			"1", 1, "L", false, 0, "")
	}
}

// fitText truncates s (with "...") so its rendered width at the current
// font stays within maxW mm. Character-count truncation overflows for wide
// glyphs — measure instead.
func fitText(pdf *fpdf.Fpdf, s string, maxW float64) string {
	if pdf.GetStringWidth(s) <= maxW {
		return s
	}
	// Binary search the longest prefix that fits with the ellipsis.
	lo, hi := 0, len(s)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if pdf.GetStringWidth(s[:mid]+"...") <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return s[:lo] + "..."
}

// filterCurrency keeps only rows in one currency.
func filterCurrency(txns []store.Transaction, code string) []store.Transaction {
	var out []store.Transaction
	for _, t := range txns {
		if t.CurrencyCode == code {
			out = append(out, t)
		}
	}
	return out
}
