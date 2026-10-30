package bot

import (
	"context"
	"fmt"
	"strings"
)

// handleOptions renders the DB-stored command catalog, grouped by section.
// The table is editable at runtime (option_help), so new features can be
// documented without a deploy.
func (d *Dispatcher) handleOptions(ctx context.Context, chatID int64) (outcome string) {
	outcome = "error"
	opts, err := d.store.ListOptionHelp(ctx)
	if err != nil {
		d.log.Error("list option help failed", "err", err)
		d.reply(ctx, chatID, "Failed to load options.")
		return
	}
	if len(opts) == 0 {
		d.reply(ctx, chatID, "No options documented.")
		return "empty"
	}
	var b strings.Builder
	b.WriteString("Financio - all features:\n")
	section := ""
	for _, o := range opts {
		if o.Section != section {
			section = o.Section
			fmt.Fprintf(&b, "\n%s:\n", section)
		}
		fmt.Fprintf(&b, "- %s\n  %s\n", o.Command, o.Description)
	}
	if err := d.tg.SendMessages(chatID, []string{b.String()}); err != nil {
		d.log.Error("send options failed", "err", err)
		return
	}
	return "options"
}
