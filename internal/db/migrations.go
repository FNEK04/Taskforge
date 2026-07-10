package db

import (
	"database/sql"

	"go.uber.org/zap"
)

const schemaSQL = `
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS tenants (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    api_key TEXT NOT NULL UNIQUE,
    rate_limits TEXT NOT NULL DEFAULT '{}',
    paused INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    type TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    priority INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    dag_id TEXT,
    dag_run_id TEXT,
    idempotency_key TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    last_error TEXT,
    scheduled_at TEXT,
    started_at TEXT,
    completed_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_jobs_tenant_status ON jobs(tenant_id, status);
CREATE INDEX IF NOT EXISTS idx_jobs_type_status ON jobs(type, status);
CREATE INDEX IF NOT EXISTS idx_jobs_idempotency ON jobs(tenant_id, idempotency_key);
CREATE INDEX IF NOT EXISTS idx_jobs_dag_run ON jobs(dag_run_id);
CREATE INDEX IF NOT EXISTS idx_jobs_scheduled ON jobs(scheduled_at);

CREATE TABLE IF NOT EXISTS job_dependencies (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    depends_on_job_id TEXT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    UNIQUE(job_id, depends_on_job_id)
);

CREATE INDEX IF NOT EXISTS idx_job_deps_job ON job_dependencies(job_id);
CREATE INDEX IF NOT EXISTS idx_job_deps_depends ON job_dependencies(depends_on_job_id);

CREATE TABLE IF NOT EXISTS dag_definitions (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    definition TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS dag_runs (
    id TEXT PRIMARY KEY,
    dag_id TEXT NOT NULL REFERENCES dag_definitions(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    status TEXT NOT NULL DEFAULT 'running',
    job_statuses TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cron_jobs (
    id TEXT PRIMARY KEY,
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    type TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}',
    cron_expr TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS dlq (
    id TEXT PRIMARY KEY,
    job_id TEXT NOT NULL REFERENCES jobs(id),
    tenant_id TEXT NOT NULL REFERENCES tenants(id),
    reason TEXT NOT NULL,
    failed_at TEXT NOT NULL,
    replayed_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_dlq_tenant ON dlq(tenant_id);
`

func RunMigrations(db *sql.DB, log *zap.Logger) error {
	_, err := db.Exec(schemaSQL)
	if err != nil {
		return err
	}
	log.Info("sqlite migrations completed")
	return nil
}
