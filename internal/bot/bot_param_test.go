package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/router"
)

func TestRouterClassifiesParam(t *testing.T) {
	for _, in := range []string{"/param", "/param list", "/param set openai_model gpt-6-luna"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentParam {
			t.Errorf("%q => %v, want param", in, got.Kind)
		}
	}
}

func TestParamSetShowList(t *testing.T) {
	fs := &fakeStore{sysparams: map[string]string{"openai_model": "gpt-6-luna"}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()

	d.Handle(ctx, 1, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/param list"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "openai_model = gpt-6-luna") {
		t.Errorf("list reply: %q", last)
	}

	d.Handle(ctx, 2, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/param set ai_attempt_timeout_seconds 120"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "ai_attempt_timeout_seconds = 120") {
		t.Errorf("set reply: %q", last)
	}
	if fs.sysparams["ai_attempt_timeout_seconds"] != "120" {
		t.Errorf("not stored: %v", fs.sysparams)
	}

	// invalid value rejected
	n := len(sender.messages)
	d.Handle(ctx, 3, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/param set ai_attempt_timeout_seconds abc"})
	if len(sender.messages) == n {
		t.Error("non-numeric timeout should be rejected")
	}

	// unknown key rejected
	d.Handle(ctx, 4, 1, 55, "Rifi", "Anggriawan", router.Message{Text: "/param set evil x"})
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Keys:") {
		t.Errorf("unknown key reply: %q", last)
	}
}
