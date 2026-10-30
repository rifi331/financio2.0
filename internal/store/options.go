package store

import (
	"context"
	"errors"
)

// OptionHelp is one /options catalog row.
type OptionHelp struct {
	Command     string
	Description string
	Section     string
	SortOrder   int
}

// ListOptionHelp returns the full command catalog ordered by section then
// sort_order.
func (s *Store) ListOptionHelp(ctx context.Context) ([]OptionHelp, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT command, description, section, sort_order
		FROM option_help
		ORDER BY section, sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OptionHelp
	for rows.Next() {
		var o OptionHelp
		if err := rows.Scan(&o.Command, &o.Description, &o.Section, &o.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// SetOptionHelp upserts one catalog row.
func (s *Store) SetOptionHelp(ctx context.Context, command, description, section string, sortOrder int) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO option_help (command, description, section, sort_order)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (command) DO UPDATE SET
			description = EXCLUDED.description, section = EXCLUDED.section, sort_order = EXCLUDED.sort_order`,
		command, description, section, sortOrder)
	return err
}

// ErrOptionNotFound marks a delete of a nonexistent row.
var ErrOptionNotFound = errors.New("option not found")

// DeleteOptionHelp removes one catalog row.
func (s *Store) DeleteOptionHelp(ctx context.Context, command string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM option_help WHERE command = $1`, command)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrOptionNotFound
	}
	return nil
}
