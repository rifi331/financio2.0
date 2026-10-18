package router

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		want    IntentKind
		wantID  int64
	}{
		{"delete upper", Message{Text: "Delete 123"}, IntentDelete, 123},
		{"hapus hash", Message{Text: "hapus #7"}, IntentDelete, 7},
		{"del spaced", Message{Text: "  del 42  "}, IntentDelete, 42},
		{"recap keywords", Message{Text: "recap nov 2025"}, IntentRecap, 0},
		{"rekap sentence", Message{Text: "rekap bulan ini makan"}, IntentRecap, 0},
		{"ringkasan", Message{Text: "tolong ringkasan pengeluaran"}, IntentRecap, 0},
		{"summary", Message{Text: "spending summary please"}, IntentRecap, 0},
		{"entry", Message{Text: "10rm nasi lemak"}, IntentEntry, 0},
		{"delete inside sentence is entry", Message{Text: "I want to delete my whole budget"}, IntentEntry, 0},
		{"photo beats text", Message{Text: "recap please", HasPhoto: true}, IntentPhoto, 0},
		{"pdf document", Message{DocumentName: "statement.pdf"}, IntentPDF, 0},
		{"pdf with caption", Message{Text: "october", DocumentName: "OCTOBER.PDF"}, IntentPDF, 0},
		{"non-pdf document", Message{Text: "10rm nasi lemak", DocumentName: "notes.txt"}, IntentEntry, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.msg)
			if got.Kind != tt.want {
				t.Errorf("Classify(%+v) kind = %v, want %v", tt.msg, got.Kind, tt.want)
			}
			if got.DeleteID != tt.wantID {
				t.Errorf("DeleteID = %d, want %d", got.DeleteID, tt.wantID)
			}
		})
	}
}
