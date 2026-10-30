package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/router"
)

func TestRouterClassifiesOptions(t *testing.T) {
	for _, in := range []string{"/options", "/options@FinancioBot"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentOptions {
			t.Errorf("%q => %v", in, got.Kind)
		}
	}
	for _, in := range []string{"/options extra", "options"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind == router.IntentOptions {
			t.Errorf("%q must not be options", in)
		}
	}
}

func TestHandleOptionsRendersCatalog(t *testing.T) {
	fs := &fakeStore{}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)

	out := d.handleOptions(context.Background(), 1)

	if out != "options" {
		t.Fatalf("outcome=%s msgs=%v", out, sender.messages)
	}
	body := strings.Join(sender.messages, "\n")
	for _, want := range []string{"Entry:", "10rm nasi lemak", "Budgets:", "/budget set"} {
		if !strings.Contains(body, want) {
			t.Errorf("options missing %q:\n%s", want, body)
		}
	}
}
