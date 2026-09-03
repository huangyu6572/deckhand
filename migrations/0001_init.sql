-- 0001_init.sql
-- LocalAIHub V1 schema. Run in a single transaction.

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at INTEGER NOT NULL
);

CREATE TABLE operations (
  id                        TEXT PRIMARY KEY,
  request_id                TEXT NOT NULL UNIQUE,
  kind                      TEXT NOT NULL,
  target_ref                TEXT NOT NULL,
  resolved_target_snapshot  TEXT NOT NULL,
  state                     TEXT NOT NULL,
  error_code                TEXT,
  exit_code                 INTEGER,
  dispatched_at             INTEGER,
  started_at                INTEGER,
  finished_at               INTEGER,
  event_path                TEXT NOT NULL,
  stdout_path               TEXT,
  stderr_path               TEXT,
  next_cursor               INTEGER NOT NULL DEFAULT 0,
  output_truncated          INTEGER NOT NULL DEFAULT 0,
  version                   INTEGER NOT NULL DEFAULT 1,
  created_at                INTEGER NOT NULL
);

CREATE INDEX operations_state ON operations(state);
CREATE INDEX operations_target ON operations(target_ref);

CREATE TABLE sessions (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL,
  target_ref      TEXT NOT NULL,
  kind            TEXT NOT NULL,
  state           TEXT NOT NULL,
  event_path      TEXT NOT NULL,
  input_path      TEXT,
  output_path     TEXT,
  next_cursor     INTEGER NOT NULL DEFAULT 0,
  created_at      INTEGER NOT NULL,
  last_activity_at INTEGER NOT NULL,
  version         INTEGER NOT NULL DEFAULT 1
);

-- Open or disconnected sessions occupy the name; closed names may be reused.
CREATE UNIQUE INDEX sessions_name_active
  ON sessions(name)
  WHERE state IN ('opening', 'open', 'disconnected');

CREATE TABLE deployments (
  operation_id      TEXT PRIMARY KEY REFERENCES operations(id),
  recipe_name       TEXT NOT NULL,
  recipe_hash       TEXT NOT NULL,
  artifact_sha256   TEXT,
  current_step      TEXT NOT NULL,
  deploy_status     TEXT NOT NULL,
  rollback_state    TEXT
);

CREATE TABLE idempotency (
  target_identity      TEXT NOT NULL,
  recipe_name          TEXT NOT NULL,
  idempotency_key      TEXT NOT NULL,
  request_fingerprint  TEXT NOT NULL,
  operation_id         TEXT NOT NULL REFERENCES operations(id),
  created_at           INTEGER NOT NULL,
  PRIMARY KEY (target_identity, recipe_name, idempotency_key)
);

INSERT INTO schema_migrations(version, applied_at) VALUES (1, 0);
