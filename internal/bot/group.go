package bot

import (
	"context"
	"fmt"
	"strings"

	"github.com/rifi331/financio/internal/store"
)

const groupUsage = "Usage:\n/group list\n/group set <display name> <group name>"

// groupOf resolves the acting user's budget group. ok=false on any error:
// budget features are skipped for that message rather than falling back to
// "all groups" (which would leak other groups' budgets).
func (d *Dispatcher) groupOf(ctx context.Context, user store.User) (int64, bool) {
	gid, err := d.store.UserGroupID(ctx, user.ID)
	if err != nil || gid == 0 {
		if err != nil {
			d.log.Warn("user group lookup failed", "err", err)
		}
		return 0, false
	}
	return gid, true
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
		// Resolve display name -> telegram id via the group roster. The
		// display name is the longest known-user prefix of the arguments;
		// the remainder is the group name (both may be multi-word).
		rest := strings.TrimSpace(strings.Join(fields[2:], " "))
		var targetID int64
		groups, err := d.store.ListGroupsWithMembers(ctx)
		if err != nil {
			d.reply(ctx, chatID, "Failed to resolve user.")
			return
		}
		var bestName string
		for _, g := range groups {
			for _, m := range g.Members {
				cand := m.DisplayName
				if len(cand) > len(bestName) && strings.HasPrefix(strings.ToLower(rest), strings.ToLower(cand)) {
					bestName = cand
					targetID = m.ID
				}
			}
		}
		groupName := strings.TrimSpace(rest[len(bestName):])
		if targetID == 0 || groupName == "" {
			d.reply(ctx, chatID, "No known user named that (they must have messaged the bot at least once).\n"+groupUsage)
			return
		}
		displayName := bestName
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

// mustGroup is groupOf for command paths: 0 when unresolvable (group 0
// matches no rows, so set/delete/list are safe no-ops).
func (d *Dispatcher) mustGroup(ctx context.Context, user store.User) int64 {
	gid, _ := d.groupOf(ctx, user)
	return gid
}
