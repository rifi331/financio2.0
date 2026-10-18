package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// SysParam is one runtime-tunable key/value pair.
type SysParam struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

// GetSysParam returns the stored value for key; ok=false means use the
// built-in default.
func (s *Store) GetSysParam(ctx context.Context, key string) (string, bool, error) {
	var v string
	err := s.pool.QueryRow(ctx,
		`SELECT value FROM sysparam WHERE key = $1`, key).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

// SetSysParam upserts a parameter.
func (s *Store) SetSysParam(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sysparam (key, value, updated_at) VALUES ($1, $2, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}

// ListSysParams returns all parameters ordered by key.
func (s *Store) ListSysParams(ctx context.Context) ([]SysParam, error) {
	rows, err := s.pool.Query(ctx, `SELECT key, value, updated_at FROM sysparam ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SysParam
	for rows.Next() {
		var p SysParam
		if err := rows.Scan(&p.Key, &p.Value, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
