package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/store"
)

var validParamKeys = []string{"openai_model", "ai_attempt_timeout_seconds", "message_timeout_seconds"}

func validParamKey(k string) bool {
	for _, n := range validParamKeys {
		if n == k {
			return true
		}
	}
	return false
}

// handleParam implements the /param admin command:
//
//	/param                    — list params and values
//	/param show <key>
//	/param set <key> <value>  — openai_model applies immediately; timeouts on restart
func (d *Dispatcher) handleParam(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	fields := strings.Fields(strings.TrimSpace(text))
	sub := "list"
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}

	usage := "Usage:\n/param list\n/param show <key>\n/param set <key> <value>\nKeys: " + strings.Join(validParamKeys, ", ")

	switch sub {
	case "list":
		params, err := d.store.ListSysParams(ctx)
		if err != nil {
			d.log.Error("list sysparams failed", "err", err)
			d.reply(ctx, chatID, "Failed to list params.")
			return
		}
		var b strings.Builder
		b.WriteString("System params in DB:\n")
		for _, p := range params {
			fmt.Fprintf(&b, "• %s = %s (updated %s)\n", p.Key, p.Value, p.UpdatedAt.In(d.tz).Format("2006-01-02 15:04"))
		}
		d.reply(ctx, chatID, b.String())
		return "param_list"

	case "show":
		if len(fields) < 3 || !validParamKey(fields[2]) {
			d.reply(ctx, chatID, usage)
			return
		}
		v, ok, err := d.store.GetSysParam(ctx, fields[2])
		if err != nil {
			d.reply(ctx, chatID, "Failed to load param.")
			return
		}
		if !ok {
			d.reply(ctx, chatID, fmt.Sprintf("%s: not set, using built-in default.", fields[2]))
			return "param_show"
		}
		d.reply(ctx, chatID, fmt.Sprintf("%s = %s", fields[2], v))
		return "param_show"

	case "set":
		if len(fields) < 4 || !validParamKey(fields[2]) {
			d.reply(ctx, chatID, usage)
			return
		}
		key, val := fields[2], fields[3]
		switch key {
		case "ai_attempt_timeout_seconds", "message_timeout_seconds":
			n, err := strconv.Atoi(val)
			if err != nil || n < 5 || n > 900 {
				d.reply(ctx, chatID, "Timeout must be a number of seconds between 5 and 900.")
				return
			}
		case "openai_model":
			if strings.TrimSpace(val) == "" {
				d.reply(ctx, chatID, usage)
				return
			}
		}
		if err := d.store.SetSysParam(ctx, key, val); err != nil {
			d.log.Error("set sysparam failed", "err", err)
			d.reply(ctx, chatID, "Failed to save param.")
			return
		}
		effect := "applies after the next restart"
		if key == "openai_model" {
			if ms, ok := d.ai.(interface{ SetModel(string) }); ok {
				ms.SetModel(val)
				effect = "applies immediately"
			}
		}
		d.reply(ctx, chatID, fmt.Sprintf("OK: %s = %s (%s).", key, val, effect))
		return "param_set"
	}

	d.reply(ctx, chatID, usage)
	return
}

// messageTimeout resolves the per-message timeout: the sysparam value when
// set and parseable, otherwise the compiled default.
func (d *Dispatcher) messageTimeout() time.Duration {
	if d.PerMessageTimeout > 0 {
		return d.PerMessageTimeout
	}
	return perMessageTimeout
}
