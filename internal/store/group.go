package store

import (
	"context"
)

// GroupWithMembers is a budget group and its members.
type GroupWithMembers struct {
	GroupID int64
	Name    string
	Members []User
}

// ListGroupsWithMembers returns every group with its active members.
func (s *Store) ListGroupsWithMembers(ctx context.Context) ([]GroupWithMembers, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT g.id, g.name, u.id, u.display_name
		FROM budget_group g
		LEFT JOIN app_user u ON u.budget_group_id = g.id AND u.is_active
		ORDER BY g.name, u.display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]*GroupWithMembers{}
	var order []int64
	for rows.Next() {
		var gid int64
		var name, dn string
		var uid *int64
		if err := rows.Scan(&gid, &name, &uid, &dn); err != nil {
			return nil, err
		}
		g, ok := byID[gid]
		if !ok {
			g = &GroupWithMembers{GroupID: gid, Name: name}
			byID[gid] = g
			order = append(order, gid)
		}
		if uid != nil {
			g.Members = append(g.Members, User{ID: *uid, DisplayName: dn, IsActive: true})
		}
	}
	out := make([]GroupWithMembers, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out, nil
}

// SetUserGroup moves a user into a group, creating the group when new.
func (s *Store) SetUserGroup(ctx context.Context, telegramID int64, groupName string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budget_group (name) VALUES ($2)
		ON CONFLICT (name) DO NOTHING`, groupName)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE app_user SET budget_group_id = (SELECT id FROM budget_group WHERE name = $2)
		WHERE id = $1`, telegramID, groupName)
	return err
}

// UserGroupID returns the acting user's budget group.
func (s *Store) UserGroupID(ctx context.Context, telegramID int64) (int64, error) {
	var gid int64
	err := s.pool.QueryRow(ctx,
		`SELECT budget_group_id FROM app_user WHERE id = $1`, telegramID).Scan(&gid)
	return gid, err
}

// EnsureUserGroup assigns a user their solo group on first message (named
// after them) so new users' budgets are isolated by default.
func (s *Store) EnsureUserGroup(ctx context.Context, telegramID int64, displayName string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budget_group (name) VALUES ($2)
		ON CONFLICT (name) DO NOTHING`, displayName)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE app_user SET budget_group_id = COALESCE(budget_group_id,
			(SELECT id FROM budget_group WHERE name = $2))
		WHERE id = $1`, telegramID, displayName)
	return err
}

// GroupMemberIDs lists the telegram ids of a group's active members.
func (s *Store) GroupMemberIDs(ctx context.Context, groupID int64) ([]int64, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id FROM app_user WHERE budget_group_id = $1 AND is_active`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
