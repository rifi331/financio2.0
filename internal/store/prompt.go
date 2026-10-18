package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Prompt is one editable AI system-prompt template.
type Prompt struct {
	Name      string
	Template  string
	UpdatedAt time.Time
}

// GetPrompt returns the stored template for name. ok=false falls back to the
// compiled-in default.
func (s *Store) GetPrompt(ctx context.Context, name string) (string, bool, error) {
	var t string
	err := s.pool.QueryRow(ctx,
		`SELECT template FROM prompt WHERE name = $1`, name).Scan(&t)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return t, true, nil
}

// SetPrompt upserts a prompt template.
func (s *Store) SetPrompt(ctx context.Context, name, template string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO prompt (name, template, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (name) DO UPDATE SET template = EXCLUDED.template, updated_at = now()`,
		name, template)
	return err
}

// DeletePrompt removes a customization (bot falls back to the default).
func (s *Store) DeletePrompt(ctx context.Context, name string) (bool, error) {
	ct, err := s.pool.Exec(ctx, `DELETE FROM prompt WHERE name = $1`, name)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ListPrompts returns all stored prompts, ordered by name.
func (s *Store) ListPrompts(ctx context.Context) ([]Prompt, error) {
	rows, err := s.pool.Query(ctx, `SELECT name, template, updated_at FROM prompt ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Prompt
	for rows.Next() {
		var p Prompt
		if err := rows.Scan(&p.Name, &p.Template, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
