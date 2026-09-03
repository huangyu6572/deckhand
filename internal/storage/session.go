package storage

import (
	"context"
	"database/sql"

	"localaihub/internal/wire"
)

type Session struct {
	ID             string
	Name           string
	TargetRef      string
	Kind           string
	State          string
	EventPath      string
	InputPath      string
	OutputPath     string
	NextCursor     int64
	CreatedAt      int64
	LastActivityAt int64
	Version        int64
}

const sessCols = `id, name, target_ref, kind, state, event_path, input_path, output_path, next_cursor, created_at, last_activity_at, version`

func (d *DB) InsertSession(ctx context.Context, s *Session) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO sessions(
		id, name, target_ref, kind, state, event_path, input_path, output_path, next_cursor, created_at, last_activity_at, version)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.ID, s.Name, s.TargetRef, s.Kind, s.State, s.EventPath, nullStr(s.InputPath), nullStr(s.OutputPath),
		s.NextCursor, s.CreatedAt, s.LastActivityAt, s.Version,
	)
	if err != nil {
		if isUnique(err) {
			return wire.E("INVALID_ARGUMENT", "duplicate session name")
		}
		return err
	}
	return nil
}

func (d *DB) UpdateSession(ctx context.Context, s *Session) error {
	now := NowUS()
	s.LastActivityAt = now
	res, err := d.sql.ExecContext(ctx, `UPDATE sessions SET name=?, target_ref=?, kind=?, state=?, event_path=?,
		input_path=?, output_path=?, next_cursor=?, last_activity_at=?, version=version+1
		WHERE id=? AND version=?`,
		s.Name, s.TargetRef, s.Kind, s.State, s.EventPath, nullStr(s.InputPath), nullStr(s.OutputPath),
		s.NextCursor, now, s.ID, s.Version)
	if err != nil {
		if isUnique(err) {
			return wire.E("INVALID_ARGUMENT", "duplicate session name")
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return wire.E("STORAGE_ERROR", "session cas conflict")
	}
	s.Version++
	return nil
}

func (d *DB) SetSessionState(ctx context.Context, id, state string, cursor int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE sessions SET state=?, next_cursor=?, last_activity_at=?, version=version+1 WHERE id=?`,
		state, cursor, NowUS(), id)
	return err
}

func (d *DB) GetSession(ctx context.Context, id string) (*Session, error) {
	s, err := scanSess(d.sql.QueryRowContext(ctx, `SELECT `+sessCols+` FROM sessions WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (d *DB) GetActiveSessionByName(ctx context.Context, name string) (*Session, error) {
	s, err := scanSess(d.sql.QueryRowContext(ctx, `SELECT `+sessCols+` FROM sessions WHERE name=? AND state IN ('opening','open','disconnected')`, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (d *DB) ListRecoverableSessions(ctx context.Context) ([]Session, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+sessCols+` FROM sessions WHERE state IN ('opening','open','disconnected')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		s, err := scanSess(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func scanSess(row rowScanner) (*Session, error) {
	var s Session
	var in, out sql.NullString
	if err := row.Scan(&s.ID, &s.Name, &s.TargetRef, &s.Kind, &s.State, &s.EventPath, &in, &out,
		&s.NextCursor, &s.CreatedAt, &s.LastActivityAt, &s.Version); err != nil {
		return nil, err
	}
	s.InputPath = in.String
	s.OutputPath = out.String
	return &s, nil
}
