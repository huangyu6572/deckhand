package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"localaihub/internal/wire"
	"localaihub/migrations"
)

type DB struct {
	sql *sql.DB
}

type Operation struct {
	ID              string
	RequestID       string
	Kind            string
	TargetRef       string
	ResolvedJSON    string
	State           string
	ErrorCode       string
	ExitCode        *int
	DispatchedAt    *int64
	StartedAt       *int64
	FinishedAt      *int64
	EventPath       string
	StdoutPath      string
	StderrPath      string
	NextCursor      int64
	OutputTruncated bool
	Version         int64
	CreatedAt       int64
}

type Deployment struct {
	OperationID    string
	RecipeName     string
	RecipeHash     string
	ArtifactSHA256 string
	CurrentStep    string
	DeployStatus   string
	RollbackState  string
}

func Open(dataDir string) (*DB, error) {
	dir := filepath.Join(dataDir, "db")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "hub.sqlite")
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)"
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&n)
	if err == nil && n > 0 {
		return nil
	}
	_, err = d.sql.Exec(migrations.InitSQL)
	return err
}

func NowUS() int64 { return time.Now().UTC().UnixMicro() }

func (d *DB) InsertOperation(ctx context.Context, op *Operation) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO operations(
		id, request_id, kind, target_ref, resolved_target_snapshot, state, error_code, exit_code,
		dispatched_at, started_at, finished_at, event_path, stdout_path, stderr_path, next_cursor,
		output_truncated, version, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		op.ID, op.RequestID, op.Kind, op.TargetRef, op.ResolvedJSON, op.State, nullStr(op.ErrorCode),
		op.ExitCode, op.DispatchedAt, op.StartedAt, op.FinishedAt, op.EventPath, nullStr(op.StdoutPath),
		nullStr(op.StderrPath), op.NextCursor, boolInt(op.OutputTruncated), op.Version, op.CreatedAt,
	)
	if err != nil {
		if isUnique(err) {
			return wire.E("INVALID_ARGUMENT", "duplicate request_id")
		}
		return err
	}
	return nil
}

func (d *DB) GetByRequest(ctx context.Context, requestID string) (*Operation, error) {
	op, err := d.get(ctx, `SELECT `+opCols+` FROM operations WHERE request_id=?`, requestID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return op, err
}

func (d *DB) Get(ctx context.Context, id string) (*Operation, error) {
	op, err := d.get(ctx, `SELECT `+opCols+` FROM operations WHERE id=?`, id)
	if err == sql.ErrNoRows {
		return nil, wire.E("JOB_NOT_FOUND", "unknown job")
	}
	return op, err
}

func (d *DB) ListJobs(ctx context.Context, limit int) ([]Operation, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	rows, err := d.sql.QueryContext(ctx, `SELECT `+opCols+` FROM operations ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		op, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

func (d *DB) CASState(ctx context.Context, id string, version int64, state string, extra func(*Operation)) error {
	op, err := d.Get(ctx, id)
	if err != nil {
		return err
	}
	if extra != nil {
		extra(op)
	}
	op.State = state
	res, err := d.sql.ExecContext(ctx, `UPDATE operations SET state=?, error_code=?, exit_code=?, dispatched_at=?,
		started_at=?, finished_at=?, next_cursor=?, output_truncated=?, version=version+1
		WHERE id=? AND version=? AND (finished_at IS NULL OR ? IN (state))`,
		op.State, nullStr(op.ErrorCode), op.ExitCode, op.DispatchedAt, op.StartedAt, op.FinishedAt,
		op.NextCursor, boolInt(op.OutputTruncated), id, version, state)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return wire.E("STORAGE_ERROR", "cas conflict")
	}
	return nil
}

func (d *DB) UpdateRunning(ctx context.Context, id string, version int64, dispatched, started int64, cursor int64) error {
	res, err := d.sql.ExecContext(ctx, `UPDATE operations SET state='running', dispatched_at=?, started_at=?, next_cursor=?, version=version+1
		WHERE id=? AND version=? AND finished_at IS NULL`, dispatched, started, cursor, id, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return wire.E("STORAGE_ERROR", "cas conflict")
	}
	return nil
}

func (d *DB) Finish(ctx context.Context, id string, version int64, state, errCode string, exit *int, cursor int64, truncated bool) error {
	now := NowUS()
	res, err := d.sql.ExecContext(ctx, `UPDATE operations SET state=?, error_code=?, exit_code=?, finished_at=?, next_cursor=?, output_truncated=?, version=version+1
		WHERE id=? AND version=? AND finished_at IS NULL`,
		state, nullStr(errCode), exit, now, cursor, boolInt(truncated), id, version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return wire.E("STORAGE_ERROR", "cas conflict")
	}
	return nil
}

func (d *DB) SetCursor(ctx context.Context, id string, cursor int64) error {
	_, err := d.sql.ExecContext(ctx, `UPDATE operations SET next_cursor=? WHERE id=?`, cursor, id)
	return err
}

func (d *DB) Incomplete(ctx context.Context) ([]Operation, error) {
	rows, err := d.sql.QueryContext(ctx, `SELECT `+opCols+` FROM operations WHERE finished_at IS NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Operation
	for rows.Next() {
		op, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *op)
	}
	return out, rows.Err()
}

func (d *DB) PutDeployment(ctx context.Context, dep Deployment) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO deployments(operation_id, recipe_name, recipe_hash, artifact_sha256, current_step, deploy_status, rollback_state)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(operation_id) DO UPDATE SET current_step=excluded.current_step, deploy_status=excluded.deploy_status, rollback_state=excluded.rollback_state`,
		dep.OperationID, dep.RecipeName, dep.RecipeHash, nullStr(dep.ArtifactSHA256), dep.CurrentStep, dep.DeployStatus, nullStr(dep.RollbackState))
	return err
}

func (d *DB) GetDeployment(ctx context.Context, opID string) (*Deployment, error) {
	row := d.sql.QueryRowContext(ctx, `SELECT operation_id, recipe_name, recipe_hash, artifact_sha256, current_step, deploy_status, rollback_state FROM deployments WHERE operation_id=?`, opID)
	var dep Deployment
	var art, rb sql.NullString
	if err := row.Scan(&dep.OperationID, &dep.RecipeName, &dep.RecipeHash, &art, &dep.CurrentStep, &dep.DeployStatus, &rb); err != nil {
		return nil, err
	}
	dep.ArtifactSHA256 = art.String
	dep.RollbackState = rb.String
	return &dep, nil
}

func (d *DB) IdempotencyGet(ctx context.Context, ident, recipe, key string) (fingerprint, opID string, err error) {
	row := d.sql.QueryRowContext(ctx, `SELECT request_fingerprint, operation_id FROM idempotency WHERE target_identity=? AND recipe_name=? AND idempotency_key=?`, ident, recipe, key)
	err = row.Scan(&fingerprint, &opID)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return fingerprint, opID, err
}

func (d *DB) IdempotencyPut(ctx context.Context, ident, recipe, key, fp, opID string) error {
	_, err := d.sql.ExecContext(ctx, `INSERT INTO idempotency(target_identity, recipe_name, idempotency_key, request_fingerprint, operation_id, created_at) VALUES(?,?,?,?,?,?)`,
		ident, recipe, key, fp, opID, NowUS())
	return err
}

func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

const opCols = `id, request_id, kind, target_ref, resolved_target_snapshot, state, error_code, exit_code,
		dispatched_at, started_at, finished_at, event_path, stdout_path, stderr_path, next_cursor, output_truncated, version, created_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func (d *DB) get(ctx context.Context, q string, arg any) (*Operation, error) {
	return scanOp(d.sql.QueryRowContext(ctx, q, arg))
}

func scanOp(row rowScanner) (*Operation, error) {
	var op Operation
	var errCode, stdout, stderr sql.NullString
	var exit sql.NullInt64
	var disp, start, fin sql.NullInt64
	var trunc int
	if err := row.Scan(&op.ID, &op.RequestID, &op.Kind, &op.TargetRef, &op.ResolvedJSON, &op.State, &errCode, &exit,
		&disp, &start, &fin, &op.EventPath, &stdout, &stderr, &op.NextCursor, &trunc, &op.Version, &op.CreatedAt); err != nil {
		return nil, err
	}
	op.ErrorCode = errCode.String
	op.StdoutPath = stdout.String
	op.StderrPath = stderr.String
	if exit.Valid {
		v := int(exit.Int64)
		op.ExitCode = &v
	}
	if disp.Valid {
		v := disp.Int64
		op.DispatchedAt = &v
	}
	if start.Valid {
		v := start.Int64
		op.StartedAt = &v
	}
	if fin.Valid {
		v := fin.Int64
		op.FinishedAt = &v
	}
	op.OutputTruncated = trunc != 0
	return &op, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func isUnique(err error) bool {
	return err != nil && (contains(err.Error(), "UNIQUE") || contains(err.Error(), "unique"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && (indexOf(s, sub) >= 0))
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func Terminal(state string) bool {
	switch state {
	case "succeeded", "failed", "timed_out", "cancelled", "execution_unknown":
		return true
	default:
		return false
	}
}
