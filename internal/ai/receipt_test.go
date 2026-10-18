package ai

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func receiptInput() ReceiptInput {
	return ReceiptInput{
		CurrentDate:     "2026-10-18",
		Categories:      []string{"Groceries", "Meals"},
		Currencies:      []string{"MYR", "IDR"},
		DefaultCurrency: "MYR",
	}
}

// TestParsePhotoReceipt checks the vision request shape (base64 data-URL image
// part, shared receipt schema wrapping the entry schema) and the decode of a
// multi-row statement response.
func TestParsePhotoReceipt(t *testing.T) {
	body := ParsedReceipt{Transactions: []ParsedEntry{
		{IsTransaction: true, Item: "Giant", Category: "Groceries", Direction: "expense", Amount: 16.78, Currency: "MYR", Date: "2026-10-14"},
		{IsTransaction: true, Item: "Tolls", Category: "Meals", Direction: "", Amount: 2.30, Currency: "MYR", Date: "2026-10-15"},
	}}
	raw, _ := json.Marshal(body)

	var gotReq ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(okBody(string(raw))))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "gpt-4o-mini")
	image := []byte("fake-jpeg-bytes")
	entries, err := c.ParsePhotoReceipt(t.Context(), image, receiptInput())
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[1].Direction != "expense" {
		t.Errorf("empty direction not defaulted to expense: %q", entries[1].Direction)
	}

	// Vision message: user content must be a [text, image_url] array with a
	// base64 data URL of the exact bytes we passed.
	// The request round-tripped through JSON, so re-marshal for stable typing.
	partsJSON, _ := json.Marshal(gotReq.Messages[1].Content)
	var parts []ContentPart
	_ = json.Unmarshal(partsJSON, &parts)
	if len(parts) != 2 {
		t.Fatalf("user content = %s, want 2 content parts", partsJSON)
	}
	if parts[0].Type != "text" || parts[0].Text != "[Current date: 2026-10-18]" {
		t.Errorf("text part = %+v", parts[0])
	}
	if parts[1].Type != "image_url" || parts[1].ImageURL == nil ||
		parts[1].ImageURL.URL != "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString(image) {
		t.Errorf("image part = %+v", parts[1])
	}

	// Schema: { transactions: [ entry schema ] }, strict.
	j, _ := json.Marshal(gotReq.ResponseFormat.JSONSchema.Schema)
	s := string(j)
	if !strings.Contains(s, `"transactions"`) || !strings.Contains(s, `"is_transaction"`) ||
		!strings.Contains(s, `"details"`) || !strings.Contains(s, `"additionalProperties":false`) {
		t.Errorf("receipt schema does not wrap the entry schema: %s", s)
	}
	if !gotReq.ResponseFormat.JSONSchema.Strict {
		t.Error("schema not strict")
	}

	sys, _ := gotReq.Messages[0].Content.(string)
	for _, want := range []string{"ONE", "transaction per row", "MYR", "dates printed on the document"} {
		if !strings.Contains(sys, want) {
			t.Errorf("photo system prompt missing %q", want)
		}
	}
}

// TestParsePhotoReceiptSingleDecodes checks a single-receipt fixture.
func TestParsePhotoReceiptSingleDecodes(t *testing.T) {
	body := ParsedReceipt{Transactions: []ParsedEntry{
		{IsTransaction: true, Item: "Giant", Category: "Groceries", Direction: "expense", Amount: 16.78, Currency: "MYR", Date: "2026-10-14",
			Details: []Detail{{Item: "rice", Amount: 10, Qty: 1}}},
	}}
	raw, _ := json.Marshal(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(okBody(string(raw))))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "")
	entries, err := c.ParsePhotoReceipt(t.Context(), []byte("img"), receiptInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || len(entries[0].Details) != 1 || entries[0].Details[0].Item != "rice" {
		t.Errorf("entries = %+v", entries)
	}
}

// TestParseTextReceiptPrompt checks the 1.0 statement rules made it into the
// system prompt, and that the extracted text is the user message.
func TestParseTextReceiptPrompt(t *testing.T) {
	raw, _ := json.Marshal(ParsedReceipt{})
	var gotReq ChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		_, _ = w.Write([]byte(okBody(string(raw))))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "k", "")
	if _, err := c.ParseTextReceipt(t.Context(), "STATEMENT TEXT", receiptInput()); err != nil {
		t.Fatal(err)
	}

	sys, _ := gotReq.Messages[0].Content.(string)
	for _, want := range []string{"Duitnow QR", "Card Payment", "RFID", "SKIP", "Receive from Wallet", "transactionDate"} {
		if !strings.Contains(sys, want) {
			t.Errorf("pdf system prompt missing %q", want)
		}
	}
	if user, _ := gotReq.Messages[1].Content.(string); !strings.Contains(user, "STATEMENT TEXT") {
		t.Errorf("pdf text not in user message: %q", user)
	}
}
