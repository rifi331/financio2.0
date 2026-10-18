package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/ai"
	"github.com/rifi331/financio/internal/router"
)

func TestRouterClassifiesPrompt(t *testing.T) {
	for _, in := range []string{"/prompt", "/prompt list", "/prompt set entry hello @FinancioBot"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentPrompt {
			t.Errorf("%q => %v, want prompt", in, got.Kind)
		}
	}
}

func TestPromptSetShowReset(t *testing.T) {
	fs := &fakeStore{}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()

	// set
	d.Handle(ctx, 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/prompt set entry Be terse. Categories: {{categories}}"})
	if len(sender.messages) == 0 || !strings.Contains(sender.messages[0], "entry updated") {
		t.Fatalf("set reply wrong: %v", sender.messages)
	}
	if got, ok := fs.prompts["entry"]; !ok || !strings.Contains(got, "{{categories}}") {
		t.Errorf("stored template wrong: %q ok=%v", got, ok)
	}

	// show
	d.Handle(ctx, 2, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/prompt show entry"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Be terse") {
		t.Errorf("show reply: %q", last)
	}

	// invalid name rejected
	n := len(sender.messages)
	d.Handle(ctx, 3, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/prompt set evilname hack"})
	if len(sender.messages) == n {
		t.Error("invalid name should be rejected")
	}

	// reset
	d.Handle(ctx, 4, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/prompt reset entry"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "reset to the built-in default") {
		t.Errorf("reset reply: %q", last)
	}
	if _, ok := fs.prompts["entry"]; ok {
		t.Error("prompt not deleted")
	}
}

// A DB template reaches the AI: set the entry template, then send an entry
// and assert the fake AI received the substituted prompt is not needed here —
// but the template lookup must not break the normal flow.
func TestPromptOverrideDoesNotBreakEntry(t *testing.T) {
	fs := &fakeStore{prompts: map[string]string{"entry": "Custom {{categories}} prompt"}}
	fai := &fakeAI{responses: map[string]ai.ParsedEntry{
		"10rm nasi lemak": {IsTransaction: true, Item: "nasi lemak", Category: "Meals", Amount: 10, Currency: "MYR", Date: "2026-10-18"},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, fai, sender)

	d.Handle(context.Background(), 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "10rm nasi lemak"})

	if len(sender.messages) == 0 || !strings.Contains(sender.messages[0], "nasi lemak") {
		t.Fatalf("entry flow broken by prompt override: %v", sender.messages)
	}
	if len(fai.requests) == 0 || fai.requests[0].SystemTemplate == "" {
		t.Errorf("template not passed to AI: %+v", fai.requests)
	}
}
