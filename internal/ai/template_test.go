package ai

import "testing"

func TestApplyTemplate(t *testing.T) {
	got := ApplyTemplate("Currency {{default_currency}} of {{currencies}} for {{categories}}.",
		map[string]string{"default_currency": "MYR", "currencies": "MYR, IDR"})
	want := "Currency MYR of MYR, IDR for {{categories}}."
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	if ApplyTemplate("no placeholders", nil) != "no placeholders" {
		t.Error("plain template should pass through")
	}
}
