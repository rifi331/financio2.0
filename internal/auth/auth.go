// Package auth implements the display-name allowlist.
package auth

import (
	"context"
	"log/slog"
	"strings"

	"github.com/rifi331/financio/internal/store"
)

// UserStore is the persistence surface auth needs.
type UserStore interface {
	GetUserByTelegramID(ctx context.Context, telegramID int64) (store.User, bool, error)
	EnsureUser(ctx context.Context, telegramID int64, displayName string) (store.User, error)
}

// Authorizer gates messages behind the configured display-name allowlist.
type Authorizer struct {
	store   UserStore
	allowed []string
	log     *slog.Logger
}

// New creates an Authorizer; allowed is the list of permitted display names.
func New(s UserStore, allowed []string) *Authorizer {
	names := make([]string, len(allowed))
	for i, a := range allowed {
		names[i] = normalize(a)
	}
	return &Authorizer{store: s, allowed: names, log: slog.Default()}
}

// Authorize decides whether a message sender may use the bot. A known active
// user passes. An unknown sender passes only if their display name matches the
// allowlist, in which case the telegram ID is permanently bound via EnsureUser.
// Everyone else is dropped silently (logged at info; never replied to).
func (a *Authorizer) Authorize(ctx context.Context, telegramID int64, firstName, lastName string) (store.User, bool, error) {
	u, found, err := a.store.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return store.User{}, false, err
	}
	if found {
		if !u.IsActive {
			a.log.Info("inactive user dropped", "user_id", telegramID)
			return store.User{}, false, nil
		}
		return u, true, nil
	}

	displayName := displayName(firstName, lastName)
	if !a.isAllowed(displayName) {
		a.log.Info("unauthorized sender dropped", "user_id", telegramID, "display_name", displayName)
		return store.User{}, false, nil
	}

	u, err = a.store.EnsureUser(ctx, telegramID, displayName)
	if err != nil {
		return store.User{}, false, err
	}
	a.log.Info("new user bound", "user_id", telegramID, "display_name", displayName)
	return u, true, nil
}

func (a *Authorizer) isAllowed(displayName string) bool {
	n := normalize(displayName)
	for _, allowed := range a.allowed {
		if n == allowed {
			return true
		}
	}
	return false
}

func displayName(firstName, lastName string) string {
	first := strings.TrimSpace(firstName)
	last := strings.TrimSpace(lastName)
	switch {
	case first == "":
		return last
	case last == "":
		return first
	default:
		return first + " " + last
	}
}

// normalize collapses whitespace and lowercases for matching.
func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
