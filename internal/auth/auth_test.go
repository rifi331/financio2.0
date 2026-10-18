package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/rifi331/financio/internal/store"
)

type fakeStore struct {
	users map[int64]store.User
}

func (f *fakeStore) GetUserByTelegramID(_ context.Context, id int64) (store.User, bool, error) {
	u, ok := f.users[id]
	return u, ok, nil
}

func (f *fakeStore) EnsureUser(_ context.Context, id int64, name string) (store.User, error) {
	if f.users == nil {
		f.users = map[int64]store.User{}
	}
	if err := errors.New("boom"); err != nil && id == -1 {
		return store.User{}, err
	}
	u := store.User{ID: id, DisplayName: name, IsActive: true}
	f.users[id] = u
	return u, nil
}

func TestAuthorize(t *testing.T) {
	fs := &fakeStore{users: map[int64]store.User{
		1: {ID: 1, DisplayName: "Alice Allowed", IsActive: true},
		2: {ID: 2, DisplayName: "Bob Banned", IsActive: false},
	}}
	a := New(fs, []string{"Alice Allowed", "Rifi Anggriawan"})
	ctx := context.Background()

	if _, ok, err := a.Authorize(ctx, 1, "Alice", "Allowed"); err != nil || !ok {
		t.Errorf("known active user: ok=%v err=%v, want ok=true", ok, err)
	}
	if _, ok, err := a.Authorize(ctx, 2, "Bob", "Banned"); err != nil || ok {
		t.Errorf("inactive user: ok=%v err=%v, want ok=false", ok, err)
	}

	// Unknown but allowed -> bound.
	u, ok, err := a.Authorize(ctx, 77, "Rifi", "Anggriawan")
	if err != nil || !ok {
		t.Fatalf("allowed newcomer: ok=%v err=%v", ok, err)
	}
	if u.ID != 77 || u.DisplayName != "Rifi Anggriawan" {
		t.Errorf("bound user = %+v", u)
	}
	if _, ok := fs.users[77]; !ok {
		t.Error("EnsureUser not called for allowed newcomer")
	}

	// Unknown and not allowed -> dropped, NOT bound.
	if _, ok, err := a.Authorize(ctx, 88, "Random", "Stranger"); err != nil || ok {
		t.Errorf("stranger: ok=%v err=%v, want ok=false", ok, err)
	}
	if _, ok := fs.users[88]; ok {
		t.Error("stranger must not be persisted")
	}

	// Case/whitespace-insensitive name matching.
	if _, ok, _ := a.Authorize(ctx, 99, "  rifi   ANGGRIAWAN ", ""); !ok {
		t.Error("normalized name should match allowlist")
	}
}
