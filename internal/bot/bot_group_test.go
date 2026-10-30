package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/rifi331/financio/internal/router"
	"github.com/rifi331/financio/internal/store"
)

func TestRouterClassifiesGroup(t *testing.T) {
	for _, in := range []string{"/group", "/group list", "/group set Rifi Anggriawan Family"} {
		if got := router.Classify(router.Message{Text: in}); got.Kind != router.IntentGroup {
			t.Errorf("%q => %v", in, got.Kind)
		}
	}
	if got := router.Classify(router.Message{Text: "group of friends"}); got.Kind == router.IntentGroup {
		t.Error("plain text must not be group")
	}
}

func TestGroupListAndSet(t *testing.T) {
	fs := &fakeStore{groups: []store.GroupWithMembers{
		{GroupID: 1, Name: "Family", Members: []store.User{{ID: 55, DisplayName: "Rifi Anggriawan"}, {ID: 66, DisplayName: "Dafina Balqis"}}},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()

	d.handleGroup(ctx, 1, store.User{ID: 55}, "/group list")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Family") || !strings.Contains(last, "Dafina") {
		t.Fatalf("list reply: %q", last)
	}

	d.handleGroup(ctx, 2, store.User{ID: 55}, "/group set Dafina Balqis Family")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "Dafina Balqis is now in group") {
		t.Fatalf("set reply: %q", last)
	}
}

func TestGroupSetUnknownUser(t *testing.T) {
	fs := &fakeStore{groups: []store.GroupWithMembers{
		{GroupID: 1, Name: "Family", Members: []store.User{{ID: 55, DisplayName: "Rifi Anggriawan"}}},
	}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	d.handleGroup(context.Background(), 1, store.User{ID: 55}, "/group set Nobody Here")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "No known user") {
		t.Fatalf("unknown user reply: %q", last)
	}
}

// Budget spend lookups and set/delete operate on the acting user's group.
func TestBudgetUsesActingUsersGroup(t *testing.T) {
	fs := &fakeStore{userGroup: 7}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	ctx := context.Background()
	user := store.User{ID: 55}

	d.handleBudget(ctx, 1, user, "/budget set Meals 500")
	if fs.lastBudgetGroup != 7 {
		t.Errorf("set targeted group %d, want 7", fs.lastBudgetGroup)
	}
	_ = d.budgetInfoLines(ctx, 7, []store.Transaction{{CategoryName: "Meals", CurrencyCode: "MYR", EntryDate: fs.rows[0].EntryDate}})
	if fs.lastSpendGroup != 7 {
		t.Errorf("spend used group %d, want 7", fs.lastSpendGroup)
	}
}

// Another group's budget is invisible to a user's /budget list.
func TestBudgetListIsGroupScoped(t *testing.T) {
	fs := &fakeStore{userGroup: 7, budgets: map[[2]string]string{{"Meals", "MYR"}: "500"},
		budgetGroupOwner: map[[2]string]int64{{"Meals", "MYR"}: 9}}
	sender := &fakeSender{}
	d := newTestDispatcher(fs, &fakeAI{}, sender)
	d.handleBudget(context.Background(), 1, store.User{ID: 55}, "/budget list")
	if last := sender.messages[len(sender.messages)-1]; !strings.Contains(last, "No budgets set") {
		t.Errorf("group 7 must not see group 9's budget: %q", last)
	}
}
