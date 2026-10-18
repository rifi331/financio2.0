package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EntryInput carries everything ParseEntry needs per request.
type EntryInput struct {
	Message         string   // raw user text
	CurrentDate     string   // YYYY-MM-DD, computed in DISPLAY_TZ by the caller
	Categories      []string // active categories from the DB
	Currencies      []string // active currency ISO codes from the DB
	DefaultCurrency string   // e.g. MYR
}

// ParsedEntry is the structured extraction result.
type ParsedEntry struct {
	IsTransaction bool     `json:"is_transaction"`
	Item          string   `json:"item"`
	Category      string   `json:"category"`
	Direction     string   `json:"direction"`
	Amount        float64  `json:"amount"`
	Currency      string   `json:"currency"`
	Date          string   `json:"date"`
	Details       []Detail `json:"details"`
}

// Detail is one line item of a receipt-like message.
type Detail struct {
	Item     string  `json:"item"`
	Amount   float64 `json:"amount"`
	Qty      int     `json:"qty"`
	Location string  `json:"location"`
}

// ParseEntry asks the model to extract a transaction from the user's message
// using strict json_schema structured output. The current date is injected by
// Go (in DISPLAY_TZ); the model never guesses it.
func (c *Client) ParseEntry(ctx context.Context, in EntryInput) (ParsedEntry, error) {
	system := systemPrompt(in)
	user := fmt.Sprintf("[Current date: %s]\n%s", in.CurrentDate, in.Message)

	req := ChatRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &ResponseFormat{
			Type:       "json_schema",
			JSONSchema: &JSONSchemaDef{Name: "expense_entry", Strict: true, Schema: entrySchema()},
		},
	}

	content, err := c.Chat(ctx, req)
	if err != nil {
		return ParsedEntry{}, err
	}

	var p ParsedEntry
	if err := json.Unmarshal([]byte(content), &p); err != nil {
		return ParsedEntry{}, fmt.Errorf("decode parsed entry: %w", err)
	}
	if p.IsTransaction && p.Direction == "" {
		p.Direction = "expense"
	}
	return p, nil
}

func systemPrompt(in EntryInput) string {
	return fmt.Sprintf(`You are a finance parsing agent for a Telegram expense tracker.
Extract item, amount, currency, date, category, and direction from the user's message.

Rules:
- Currency: if the message does not specify one, use %s. Map "rm"/"myr" -> MYR,
  "rp"/"idr" -> IDR, "sgd"/"s$" -> SGD, "$"/"usd" -> USD. Normalize shorthand
  amounts: "10rm" -> 10 MYR, "rp50k" or "50k" -> 50000. The currency must be
  exactly one of: %s.
- Date: prefer a date stated in the message ("yesterday", "kemarin",
  "kemaren lusa"/"last friday", explicit dates). Otherwise use the current
  date given in the user message. Reply with YYYY-MM-DD.
- Category: choose exactly one from: %s.
- Direction: "income" when the message is about receiving money (gaji, salary,
  bonus, refund, received, diterima, transfer masuk); otherwise "expense".
- Item: a short label for the transaction.
- details: list the individual line items when the message breaks the purchase
  down; each needs item, amount, qty (default 1), and location (may be empty).
- If the message is a greeting, chit-chat, or anything that is not a
  transaction, set is_transaction=false and leave the other fields empty.`,
		in.DefaultCurrency,
		strings.Join(in.Currencies, ", "),
		strings.Join(in.Categories, ", "))
}

func entrySchema() map[string]any {
	prop := func(t string) map[string]any { return map[string]any{"type": t} }
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required": []string{"is_transaction", "item", "category", "direction",
			"amount", "currency", "date", "details"},
		"properties": map[string]any{
			"is_transaction": prop("boolean"),
			"item":           prop("string"),
			"category":       prop("string"),
			"direction":      map[string]any{"type": "string", "enum": []string{"expense", "income"}},
			"amount":         prop("number"),
			"currency":       prop("string"),
			"date":           prop("string"),
			"details": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"item", "amount", "qty", "location"},
					"properties": map[string]any{
						"item":     prop("string"),
						"amount":   prop("number"),
						"qty":      prop("integer"),
						"location": prop("string"),
					},
				},
			},
		},
	}
}
