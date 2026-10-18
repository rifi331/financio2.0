package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// ReceiptInput carries everything the receipt parsers need per request.
type ReceiptInput struct {
	CurrentDate     string   // YYYY-MM-DD in DISPLAY_TZ
	Categories      []string // active categories from the DB
	Currencies      []string // active currency ISO codes
	DefaultCurrency string   // e.g. MYR
}

// ParsedReceipt is the { "transactions": [...] } wrapper the receipt schema
// uses: session-2's entry schema per element.
type ParsedReceipt struct {
	Transactions []ParsedEntry `json:"transactions"`
}

// ParsePhotoReceipt sends the receipt photo to a vision model and returns the
// parsed transactions (single receipt -> one transaction with line items in
// details; statement/list -> one transaction per row).
func (c *Client) ParsePhotoReceipt(ctx context.Context, image []byte, in ReceiptInput) ([]ParsedEntry, error) {
	dataURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)
	msgs := []Message{
		{Role: "system", Content: photoSystemPrompt(in)},
		{Role: "user", Content: []ContentPart{
			{Type: "text", Text: fmt.Sprintf("[Current date: %s]", in.CurrentDate)},
			{Type: "image_url", ImageURL: &ImageURL{URL: dataURL}},
		}},
	}
	return c.parseReceipt(ctx, msgs)
}

// ParseTextReceipt sends extracted PDF text to the model with the 1.0
// statement rules (keep spending rows, skip wallet top-up receive rows).
func (c *Client) ParseTextReceipt(ctx context.Context, text string, in ReceiptInput) ([]ParsedEntry, error) {
	msgs := []Message{
		{Role: "system", Content: pdfSystemPrompt(in)},
		{Role: "user", Content: fmt.Sprintf("[Current date: %s]\n%s", in.CurrentDate, text)},
	}
	return c.parseReceipt(ctx, msgs)
}

// parseReceipt is the single parser both media flows share.
func (c *Client) parseReceipt(ctx context.Context, msgs []Message) ([]ParsedEntry, error) {
	req := ChatRequest{
		Model:          c.Model,
		Messages:       msgs,
		ResponseFormat: &ResponseFormat{Type: "json_schema", JSONSchema: &JSONSchemaDef{Name: "receipt", Strict: true, Schema: receiptSchema()}},
	}
	content, err := c.Chat(ctx, req)
	if err != nil {
		return nil, err
	}
	var p ParsedReceipt
	if err := json.Unmarshal([]byte(content), &p); err != nil {
		return nil, fmt.Errorf("decode parsed receipt: %w", err)
	}
	for i := range p.Transactions {
		if p.Transactions[i].Direction == "" {
			p.Transactions[i].Direction = "expense"
		}
	}
	return p.Transactions, nil
}

func photoSystemPrompt(in ReceiptInput) string {
	return fmt.Sprintf(`You are a receipt-parsing agent for a Telegram expense tracker.
You receive a photo of a receipt or document. Classify it by one test:
does the document show ONE grand total that covers everything on it?

A) Grand total present (store receipt, restaurant bill: itemized lines
   plus a single "Total" at the bottom): return exactly ONE transaction
   whose amount is that total; list the individual line items in details
   (item, amount, qty, location).
B) No grand total — the image is a list of separate, independently dated
   transactions (transaction history screenshot, bank/e-wallet activity
   page, spending log): return one transaction per row, each with its own
   amount and the date shown for that row. Do NOT collapse these into a
   single transaction and do not stop at the first row — include every
   visible transaction row.

Rules:
- Currency: use the currency printed on the document when identifiable,
  otherwise default to %s. The currency must be exactly one of: %s.
- Dates: use dates printed on the document when present (YYYY-MM-DD);
  otherwise fall back to the current date given in the user message.
- Discounts and vouchers: apply them to the line amounts they belong to, and
  record each one in that detail line's note (e.g. "voucher -5.00").
- Payment type: when the document shows how a row was paid (Duitnow QR,
  Card Payment, RFID, cash...), record it in that line's note.
- Category: choose exactly one from: %s.
- Direction: "expense" for spending, "income" only for genuine income.
- Item: a short label for each transaction.
- Set is_transaction=true on every returned transaction. Return an empty
  transactions array only if the image contains no financial document.`,
		in.DefaultCurrency,
		strings.Join(in.Currencies, ", "),
		strings.Join(in.Categories, ", "))
}

func pdfSystemPrompt(in ReceiptInput) string {
	return fmt.Sprintf(`You are a bank-statement parsing agent for a Telegram expense tracker.
You receive plain text extracted from a bank / e-wallet statement PDF.

Rules (ported from 1.0):
- KEEP spending rows such as: Duitnow QR, Payment, Card Payment, RFID, and
  any other debit/spending line.
- SKIP receive-type rows: "Receive", "Received", "Receive from Wallet" — those
  are wallet top-ups (transfers to self), not income.
- Use the per-line transactionDate printed on the document (YYYY-MM-DD);
  when a row has no date, inherit the nearest previous row's date.
- Discounts and vouchers: apply them to the row amounts they belong to and
  record each one in that detail line's note (e.g. "voucher -5.00").
- Payment type: record the row's type (Duitnow QR, Payment, Card Payment,
  RFID...) in that line's note.
- Aggregate line details into one or many transactions: one per statement row,
  each with its own date; or a single grouped transaction when rows clearly
  belong to one purchase.
- Currency: default %s unless the document says otherwise; must be one of: %s.
- Category: choose exactly one from: %s.
- Direction: "expense" for spending rows. Never emit income for receive-type
  rows — they are skipped entirely.
- Item: a short label for each transaction.
- Set is_transaction=true on every returned transaction.`,
		in.DefaultCurrency,
		strings.Join(in.Currencies, ", "),
		strings.Join(in.Categories, ", "))
}

// receiptSchema wraps the session-2 entry schema:
// { "transactions": [ <entry schema> ] }.
func receiptSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"transactions"},
		"properties": map[string]any{
			"transactions": map[string]any{
				"type":  "array",
				"items": entrySchema(),
			},
		},
	}
}
