package bot

import (
	"context"
	"strings"
	"time"

	"github.com/rifi331/financio/internal/store"
)

// userCacheTTL bounds how stale the roster used for transfer resolution may
// be. New users message the bot at least once before they can be transfer
// targets, so an hour of staleness only delays recognizing a rename.
const userCacheTTL = time.Hour

// knownUser resolves a display name to a registered user using the cached
// roster. Exact (case-insensitive) match wins; a unique first-name match is
// accepted as a convenience.
func (d *Dispatcher) knownUser(ctx context.Context, name string) (store.User, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return store.User{}, false
	}
	users := d.cachedUsers(ctx)
	var firstMatch store.User
	firstMatches := 0
	for _, u := range users {
		switch strings.ToLower(u.DisplayName) {
		case name:
			return u, true
		}
		if strings.EqualFold(strings.Fields(u.DisplayName)[0], name) {
			firstMatch = u
			firstMatches++
		}
	}
	if firstMatches == 1 {
		return firstMatch, true
	}
	return store.User{}, false
}

func (d *Dispatcher) cachedUsers(ctx context.Context) []store.User {
	d.usersMu.Lock()
	defer d.usersMu.Unlock()
	if d.usersCache != nil && time.Since(d.usersCachedAt) < userCacheTTL {
		return d.usersCache
	}
	groups, err := d.store.ListGroupsWithMembers(ctx)
	if err != nil {
		d.log.Warn("user roster refresh failed; using stale copy", "err", err)
		if d.usersCache != nil {
			return d.usersCache
		}
		return nil
	}
	var users []store.User
	seen := map[int64]bool{}
	for _, g := range groups {
		for _, m := range g.Members {
			if !seen[m.ID] {
				seen[m.ID] = true
				users = append(users, m)
			}
		}
	}
	d.usersCache = users
	d.usersCachedAt = time.Now()
	return users
}
