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
	IntentDuplicate
	IntentReport
	IntentPrompt
	IntentParam
	IntentPhoto
	IntentPDF
	IntentHelp
)

// Message is the normalized inbound message shape the router needs.
type Message struct {
	Text           string
	HasPhoto       bool
	PhotoFileID    string // file_id of the largest photo size, "" otherwise
	DocumentName   string // file name for document messages, "" otherwise
	DocumentFileID string // file_id of the document, "" otherwise
}

// Intent is the classification result. DeleteID is set only for IntentDelete.
type Intent struct {
	Kind     IntentKind
	DeleteID int64
	Text     string
}

var deleteRe = regexp.MustCompile(`(?i)^\s*(hapus|delete|del)\s+#?(\d+)\s*$`)

var helpRe = regexp.MustCompile(`^/(start|help)(@\w+)?$`)

var promptRe = regexp.MustCompile(`^/prompt(@\w+)?(\s+.*)?$`)

var paramRe = regexp.MustCompile(`^/param(@\w+)?(\s+.*)?$`)

var duplicateRe = regexp.MustCompile(`(?i)\b(check|cek|find|cari)\s+(for\s+)?(duplicates?|duplikat)\b`)

var reportRe = regexp.MustCompile(`(?i)\b(full|complete|lengkap)\s+(report|laporan)\b|\blaporan\s+lengkap\b`)

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

	if helpRe.MatchString(strings.TrimSpace(m.Text)) {
		return Intent{Kind: IntentHelp, Text: m.Text}
	}

	if promptRe.MatchString(strings.TrimSpace(m.Text)) {
		return Intent{Kind: IntentPrompt, Text: m.Text}
	}

	if paramRe.MatchString(strings.TrimSpace(m.Text)) {
		return Intent{Kind: IntentParam, Text: m.Text}
	}

	if match := deleteRe.FindStringSubmatch(m.Text); match != nil {
		var id int64
		for _, c := range match[2] {
			id = id*10 + int64(c-'0')
		}
		return Intent{Kind: IntentDelete, DeleteID: id, Text: m.Text}
	}

	lower := strings.ToLower(m.Text)
	if reportRe.MatchString(lower) {
		return Intent{Kind: IntentReport, Text: m.Text}
	}
	if duplicateRe.MatchString(lower) {
		return Intent{Kind: IntentDuplicate, Text: m.Text}
	}
	for _, w := range recapWords {
		if strings.Contains(lower, w) {
			return Intent{Kind: IntentRecap, Text: m.Text}
		}
	}

	return Intent{Kind: IntentEntry, Text: m.Text}
}

// IntentName returns a stable label for logs.
func (k IntentKind) String() string {
	switch k {
	case IntentEntry:
		return "entry"
	case IntentDelete:
		return "delete"
	case IntentRecap:
		return "recap"
	case IntentDuplicate:
		return "duplicate"
	case IntentReport:
		return "report"
	case IntentPrompt:
		return "prompt"
	case IntentParam:
		return "param"
	case IntentPhoto:
		return "photo"
	case IntentPDF:
		return "pdf"
	case IntentHelp:
		return "help"
	}
	return "unknown"
}

// IntentName is a wrapper kept for callers that prefer a function form.
func IntentName(k IntentKind) string { return k.String() }
