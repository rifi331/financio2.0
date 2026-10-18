// Package router classifies inbound messages into intents.
package router

import (
	"regexp"
	"strings"
)

// IntentKind enumerates the supported intents.
type IntentKind int

const (
	IntentEntry IntentKind = iota
	IntentDelete
	IntentRecap
	IntentPhoto
	IntentPDF
)

// Message is the normalized inbound message shape the router needs.
type Message struct {
	Text           string
	HasPhoto       bool
	DocumentName   string // file name for document messages, "" otherwise
}

// Intent is the classification result. DeleteID is set only for IntentDelete.
type Intent struct {
	Kind     IntentKind
	DeleteID int64
	Text     string
}

var deleteRe = regexp.MustCompile(`(?i)^\s*(hapus|delete|del)\s+#?(\d+)\s*$`)

var recapWords = []string{"recap", "rekap", "summary", "ringkasan"}

// Classify maps a message to an intent. Media beats text classification; a
// sentence that merely contains "delete" is NOT IntentDelete — only the exact
// command form matches.
func Classify(m Message) Intent {
	if m.HasPhoto {
		return Intent{Kind: IntentPhoto, Text: m.Text}
	}
	if strings.HasSuffix(strings.ToLower(m.DocumentName), ".pdf") {
		return Intent{Kind: IntentPDF, Text: m.Text}
	}

	if match := deleteRe.FindStringSubmatch(m.Text); match != nil {
		var id int64
		for _, c := range match[2] {
			id = id*10 + int64(c-'0')
		}
		return Intent{Kind: IntentDelete, DeleteID: id, Text: m.Text}
	}

	lower := strings.ToLower(m.Text)
	for _, w := range recapWords {
		if strings.Contains(lower, w) {
			return Intent{Kind: IntentRecap, Text: m.Text}
		}
	}

	return Intent{Kind: IntentEntry, Text: m.Text}
}
