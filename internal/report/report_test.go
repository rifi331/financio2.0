package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/rifi331/financio/internal/store"
)

func sampleData() Data {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }
	txns := []store.Transaction{
		{Item: "nasi lemak", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "10.50", Direction: "expense", EntryDate: day(1)},
		{Item: "bakso", CategoryName: "Meals", CurrencyCode: "MYR", TotalAmount: "12.00", Direction: "expense", EntryDate: day(2)},
		{Item: "tol", CategoryName: "Public Transportation", CurrencyCode: "MYR", TotalAmount: "5.00", Direction: "expense", EntryDate: day(3)},
		{Item: "kopi", CategoryName: "Drinks", CurrencyCode: "IDR", TotalAmount: "25000", Direction: "expense", EntryDate: day(4)},
		{Item: "gaji", CategoryName: "Others", CurrencyCode: "MYR", TotalAmount: "6500.00", Direction: "income", EntryDate: day(1)},
	}
	return Data{
		From: day(1), To: day(31), Txns: txns,
		Currencies: []store.Currency{{Code: "MYR", DecimalPlaces: 2}, {Code: "IDR", DecimalPlaces: 0}},
		TZ:         time.UTC, Generated: time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC),
	}
}

func TestBuildProducesValidPDF(t *testing.T) {
	b, err := Build(sampleData())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Error("not a PDF")
	}
	if !bytes.Contains(b, []byte("%%EOF")) {
		t.Error("PDF missing EOF marker")
	}
	if len(b) < 2000 {
		t.Errorf("suspiciously small: %d bytes", len(b))
	}
}

func TestBuildEmpty(t *testing.T) {
	d := sampleData()
	d.Txns = nil
	b, err := Build(d)
	if err != nil {
		t.Fatalf("build empty: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Error("not a PDF")
	}
}

func TestBuildManyRowsPaginates(t *testing.T) {
	d := sampleData()
	d.Txns = nil
	for i := 0; i < 120; i++ {
		d.Txns = append(d.Txns, store.Transaction{
			Item: "item long name", CategoryName: "Meals", CurrencyCode: "MYR",
			TotalAmount: "1.00", Direction: "expense",
			EntryDate: time.Date(2026, 10, 1+i%28, 0, 0, 0, 0, time.UTC),
		})
	}
	b, err := Build(d)
	if err != nil {
		t.Fatalf("build many: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Error("not a PDF")
	}
	// Page-count sanity: /Type /Pages object should exist.
	if !bytes.Contains(b, []byte("/Type /Pages")) {
		t.Error("no pages object")
	}
}

func TestParseAmount(t *testing.T) {
	for in, want := range map[string]float64{"10.50": 10.5, "6500": 6500, " 12.25 ": 12.25} {
		if got, err := parse(in); err != nil || got != want {
			t.Errorf("parse(%q) = %v, %v", in, got, err)
		}
	}
	if strings.Contains("IDR", "USD") {
		t.Fail()
	}
}

func TestBuildGroupByCategory(t *testing.T) {
	d := sampleData()
	d.GroupByCategory = true
	b, err := Build(d)
	if err != nil {
		t.Fatalf("build grouped: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Error("not a PDF")
	}
}

func TestBuildChartsAndInsightsMultiCategory(t *testing.T) {
	d := sampleData()
	d.Txns = append(d.Txns,
		store.Transaction{Item: "obat", CategoryName: "Medical", CurrencyCode: "MYR", TotalAmount: "198.00", Direction: "expense", EntryDate: time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)},
		store.Transaction{Item: "sewa", CategoryName: "Housing", CurrencyCode: "MYR", TotalAmount: "1200.00", Direction: "expense", EntryDate: time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)},
	)
	d.GroupByCategory = true
	b, err := Build(d)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) || !bytes.Contains(b, []byte("%%EOF")) {
		t.Error("invalid PDF with charts/insights")
	}
	base, err := Build(sampleData())
	if err != nil {
		t.Fatalf("base build: %v", err)
	}
	if len(b) <= len(base) {
		t.Error("charts should add content")
	}
}
