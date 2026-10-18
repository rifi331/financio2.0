package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// RecapFilter is the structured recap query the model may return. It is a
// filter description only — never SQL; Go builds the parameterized query.
type RecapFilter struct {
	IsRecapRequest bool     `json:"is_recap_request"`
	DateFrom       string   `json:"date_from"` // YYYY-MM-DD
	DateTo         string   `json:"date_to"`   // YYYY-MM-DD
	Categories     []string `json:"categories"`
	Language       string   `json:"language"`
}

// ParseRecapFilter asks the model to interpret a recap request into a
// structured date/category filter. today is the current date in DISPLAY_TZ,
// injected by Go so the model never guesses dates.
func (c *Client) ParseRecapFilter(ctx context.Context, text string, today time.Time, categories []string) (RecapFilter, error) {
	system := recapSystemPrompt(categories)
	user := fmt.Sprintf("[Today's date: %s]\n%s", today.Format("2006-01-02"), text)

	req := ChatRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ResponseFormat: &ResponseFormat{
			Type:       "json_schema",
			JSONSchema: &JSONSchemaDef{Name: "recap_filter", Strict: true, Schema: recapSchema()},
		},
	}

	content, err := c.Chat(ctx, req)
	if err != nil {
		return RecapFilter{}, err
	}

	var f RecapFilter
	if err := json.Unmarshal([]byte(content), &f); err != nil {
		return RecapFilter{}, fmt.Errorf("decode recap filter: %w", err)
	}
	return f, nil
}

func recapSystemPrompt(categories []string) string {
	return fmt.Sprintf(`You interpret period requests for a Telegram expense tracker into a
structured date/category filter. The request may be a recap/summary
("recap nov 2025", "rekap bulan ini"), a duplicate check ("check duplicate
october", "cek duplikat oktober"), or a full report ("full report last
month", "laporan lengkap november") — all of them carry a period; extract it
the same way. Reply in JSON only, per the schema.

Rules:
- date_from/date_to are inclusive YYYY-MM-DD bounds derived from natural
  language, in both English and Indonesian: month names ("recap nov 2025",
  "recap sept 2020"), "today", "recap for today", "rekap bulan ini" (this
  month), "kemarin" (yesterday), "minggu ini" (this week, Monday-Sunday),
  "bulan lalu" (last month), "this year", explicit date ranges.
- A bare month name ("october", "oktober") means that whole calendar month.
- Always compute the bounds from the today date given in the user message;
  never guess or hardcode dates.
- categories: a subset (case-insensitive match) of this list: %s.
  Map synonyms loosely ("makan" -> Meals, "belanja" -> Groceries,
  "transport" -> Public Transportation). Use [] for no category filter.
- language: "ID" when the request is Indonesian, otherwise "EN".
- Set is_recap_request=false with empty fields ONLY when the message does
  not ask about any period of transactions at all (greetings, chit-chat).`,
		strings.Join(categories, ", "))
}

func recapSchema() map[string]any {
	prop := func(t string) map[string]any { return map[string]any{"type": t} }
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"is_recap_request", "date_from", "date_to", "categories", "language"},
		"properties": map[string]any{
			"is_recap_request": prop("boolean"),
			"date_from":        prop("string"),
			"date_to":          prop("string"),
			"categories": map[string]any{
				"type":  "array",
				"items": prop("string"),
			},
			"language": map[string]any{"type": "string", "enum": []string{"EN", "ID"}},
		},
	}
}
