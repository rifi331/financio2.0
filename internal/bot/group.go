package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/rifi331/financio/internal/store"
)

const groupUsage = "Usage:\n/group list\n/group set <display name> <group name>"

// groupOf resolves the acting user's budget group (0 = no group; budget
// features then behave as unset).
func (d *Dispatcher) groupOf(ctx context.Context, user store.User) int64 {
	gid, err := d.store.UserGroupID(ctx, user.ID)
	if err != nil {
		d.log.Warn("user group lookup failed", "err", err)
		return 0
	}
	return gid
}

// handleGroup implements the /group admin command.
func (d *Dispatcher) handleGroup(ctx context.Context, chatID int64, user store.User, text string) (outcome string) {
	outcome = "error"
	fields := strings.Fields(strings.TrimSpace(text))
	sub := "list"
	if len(fields) >= 2 {
		sub = strings.ToLower(fields[1])
	}

	switch sub {
	case "list":
		groups, err := d.store.ListGroupsWithMembers(ctx)
		if err != nil {
			d.reply(ctx, chatID, "Failed to list groups.")
			return
		}
		if len(groups) == 0 {
			d.reply(ctx, chatID, "No budget groups. "+groupUsage)
			return "group_list"
		}
		var b strings.Builder
		b.WriteString("Budget groups:\n")
		for _, g := range groups {
			names := make([]string, len(g.Members))
			for i, m := range g.Members {
				names[i] = m.DisplayName
			}
			fmt.Fprintf(&b, "- %s: %s\n", g.Name, strings.Join(names, ", "))
		}
		d.reply(ctx, chatID, b.String())
		return "group_list"

	case "set":
		if len(fields) < 4 {
			d.reply(ctx, chatID, groupUsage)
			return
		}
		// display name may span words until the final group name
		groupName := fields[len(fields)-1]
		displayName := strings.Join(fields[2:len(fields)-1], " ")
		if displayName == "" {
			d.reply(ctx, chatID, groupUsage)
			return
		}
		// Resolve display name -> telegram id via the group roster.
		var targetID int64
		groups, err := d.store.ListGroupsWithMembers(ctx)
		if err != nil {
			d.reply(ctx, chatID, "Failed to resolve user.")
			return
		}
		for _, g := range groups {
			for _, m := range g.Members {
				if strings.EqualFold(m.DisplayName, displayName) {
					targetID = m.ID
				}
			}
		}
		if targetID == 0 {
			d.reply(ctx, chatID, fmt.Sprintf("No known user named %q (they must have messaged the bot at least once).", displayName))
			return
		}
		if err := d.store.SetUserGroup(ctx, targetID, groupName); err != nil {
			d.reply(ctx, chatID, "Failed to set group.")
			return
		}
		d.reply(ctx, chatID, fmt.Sprintf("OK: %s is now in group %q.", displayName, groupName))
		return "group_set"
	}

	d.reply(ctx, chatID, groupUsage)
	return
}
