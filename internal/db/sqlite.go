package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/taskforge/internal"
	_ "modernc.org/sqlite"
	"go.uber.org/zap"
)

type DB struct {
	*sql.DB
}

func New(path string, maxConns int, log *zap.Logger) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	log.Info("connected to sqlite", zap.String("path", path))
	return &DB{db}, nil
}

func (d *DB) CreateJob(ctx context.Context, job *internal.Job) error {
	payload := string(job.Payload)
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `
		INSERT INTO jobs (id, tenant_id, type, payload, priority, status, dag_id, dag_run_id,
		                  idempotency_key, retry_count, max_retries, last_error, scheduled_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.TenantID, job.Type, payload, job.Priority, job.Status,
		nullStr(job.DAGID), nullStr(job.DAGRunID), nullStr(job.IdempotencyKey),
		job.RetryCount, job.MaxRetries, nullStr(job.LastError),
		formatTime(job.ScheduledAt), job.CreatedAt.Format(time.RFC3339Nano), now)
	return err
}

func (d *DB) UpdateJobStatus(ctx context.Context, id string, status internal.JobStatus, lastErr *string) error {
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status=?, last_error=?, updated_at=? WHERE id=?`,
		status, nullStr(lastErr), time.Now().Format(time.RFC3339Nano), id)
	return err
}

func (d *DB) UpdateJobStarted(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='running', started_at=?, updated_at=? WHERE id=?`,
		now, now, id)
	return err
}

func (d *DB) UpdateJobCompleted(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='completed', completed_at=?, updated_at=? WHERE id=?`,
		now, now, id)
	return err
}

func (d *DB) UpdateJobFailed(ctx context.Context, id string, retryCount int, lastErr *string, status internal.JobStatus) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status=?, retry_count=?, last_error=?, updated_at=? WHERE id=?`,
		status, retryCount, nullStr(lastErr), now, id)
	return err
}

func (d *DB) GetJob(ctx context.Context, id string) (*internal.Job, error) {
	row := d.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, payload, priority, status, dag_id, dag_run_id,
		       idempotency_key, retry_count, max_retries, last_error, scheduled_at,
		       started_at, completed_at, created_at
		FROM jobs WHERE id=?`, id)

	return scanJob(row.Scan)
}

func (d *DB) ListJobs(ctx context.Context, tenantID string, status internal.JobStatus, limit, offset int) ([]*internal.Job, error) {
	rows, err := d.QueryContext(ctx, `
		SELECT id, tenant_id, type, payload, priority, status, dag_id, dag_run_id,
		       idempotency_key, retry_count, max_retries, last_error, scheduled_at,
		       started_at, completed_at, created_at
		FROM jobs WHERE tenant_id=? AND (?='' OR status=?)
		ORDER BY created_at DESC LIMIT ? OFFSET ?`, tenantID, status, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*internal.Job
	for rows.Next() {
		j, err := scanJob(rows.Scan)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (d *DB) GetTenant(ctx context.Context, apiKey string) (*internal.Tenant, error) {
	row := d.QueryRowContext(ctx, `SELECT id, name, api_key, rate_limits, paused, created_at FROM tenants WHERE api_key=?`, apiKey)
	t := &internal.Tenant{}
	var rateLimits string
	var paused int
	var createdAt string
	err := row.Scan(&t.ID, &t.Name, &t.APIKey, &rateLimits, &paused, &createdAt)
	if err != nil {
		return nil, err
	}
	t.Paused = paused == 1
	t.RateLimits = make(map[string]internal.RateLimit)
	json.Unmarshal([]byte(rateLimits), &t.RateLimits)
	parsed, _ := time.Parse(time.RFC3339Nano, createdAt)
	t.CreatedAt = parsed
	return t, nil
}

func (d *DB) CreateTenant(ctx context.Context, t *internal.Tenant) error {
	_, err := d.ExecContext(ctx, `INSERT INTO tenants (id, name, api_key, rate_limits, paused, created_at) VALUES (?,?,?,?,?,?)`,
		t.ID, t.Name, t.APIKey, "{}", boolInt(t.Paused), t.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (d *DB) EnsureTenant(ctx context.Context, tenantID string) error {
	now := time.Now()
	_, err := d.ExecContext(ctx, `INSERT OR IGNORE INTO tenants (id, name, api_key, paused, created_at) VALUES (?,?,?,0,?)`,
		tenantID, tenantID, tenantID, now.Format(time.RFC3339Nano))
	return err
}

func (d *DB) SetTenantPaused(ctx context.Context, tenantID string, paused bool) error {
	_, err := d.ExecContext(ctx, `UPDATE tenants SET paused=? WHERE id=?`, boolInt(paused), tenantID)
	return err
}

func (d *DB) CancelJob(ctx context.Context, jobID string) error {
	res, err := d.ExecContext(ctx, `UPDATE jobs SET status='cancelled', updated_at=? WHERE id=? AND status NOT IN ('completed','cancelled','dlq')`,
		time.Now().Format(time.RFC3339Nano), jobID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("job not found or cannot be cancelled")
	}
	return nil
}

func (d *DB) GetDAGDefinition(ctx context.Context, id string) (*internal.DAGDefinition, error) {
	row := d.QueryRowContext(ctx, `SELECT id, tenant_id, name, definition, created_at FROM dag_definitions WHERE id=?`, id)
	dd := &internal.DAGDefinition{}
	var def string
	var createdAt string
	err := row.Scan(&dd.ID, &dd.TenantID, &dd.Name, &def, &createdAt)
	if err != nil {
		return nil, err
	}
	var data struct {
		Nodes []internal.DAGNode `json:"nodes"`
		Edges []internal.DAGEdge `json:"edges"`
	}
	if err := json.Unmarshal([]byte(def), &data); err != nil {
		return nil, err
	}
	dd.Nodes = data.Nodes
	dd.Edges = data.Edges
	parsed, _ := time.Parse(time.RFC3339Nano, createdAt)
	dd.CreatedAt = parsed
	return dd, nil
}

func (d *DB) ListDAGDefinitions(ctx context.Context, tenantID string) ([]*internal.DAGDefinition, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, tenant_id, name, definition, created_at FROM dag_definitions WHERE tenant_id=? ORDER BY created_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var dags []*internal.DAGDefinition
	for rows.Next() {
		dd := &internal.DAGDefinition{}
		var def string
		var createdAt string
		if err := rows.Scan(&dd.ID, &dd.TenantID, &dd.Name, &def, &createdAt); err != nil {
			return nil, err
		}
		var data struct {
			Nodes []internal.DAGNode `json:"nodes"`
			Edges []internal.DAGEdge `json:"edges"`
		}
		if err := json.Unmarshal([]byte(def), &data); err != nil {
			return nil, err
		}
		dd.Nodes = data.Nodes
		dd.Edges = data.Edges
		parsed, _ := time.Parse(time.RFC3339Nano, createdAt)
		dd.CreatedAt = parsed
		dags = append(dags, dd)
	}
	return dags, rows.Err()
}

func (d *DB) CreateDAGDefinition(ctx context.Context, dd *internal.DAGDefinition) error {
	defBytes, _ := json.Marshal(map[string]interface{}{
		"nodes": dd.Nodes,
		"edges": dd.Edges,
	})
	_, err := d.ExecContext(ctx, `INSERT INTO dag_definitions (id, tenant_id, name, definition, created_at) VALUES (?,?,?,?,?)`,
		dd.ID, dd.TenantID, dd.Name, string(defBytes), dd.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (d *DB) CreateDAGRun(ctx context.Context, run *internal.DAGRun) error {
	_, err := d.ExecContext(ctx, `INSERT INTO dag_runs (id, dag_id, tenant_id, status, job_statuses, created_at) VALUES (?,?,?,?,?,?)`,
		run.ID, run.DAGID, run.TenantID, run.Status, "{}", run.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (d *DB) GetAllCronJobs(ctx context.Context) ([]*internal.CronJob, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, tenant_id, type, payload, cron_expr, priority, max_retries, enabled, created_at FROM cron_jobs WHERE enabled=1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*internal.CronJob
	for rows.Next() {
		cj := &internal.CronJob{}
		var payload, createdAt string
		var enabled int
		if err := rows.Scan(&cj.ID, &cj.TenantID, &cj.Type, &payload, &cj.CronExpr, &cj.Priority, &cj.MaxRetries, &enabled, &createdAt); err != nil {
			return nil, err
		}
		cj.Payload = []byte(payload)
		cj.Enabled = enabled == 1
		parsed, _ := time.Parse(time.RFC3339Nano, createdAt)
		cj.CreatedAt = parsed
		jobs = append(jobs, cj)
	}
	return jobs, nil
}

func (d *DB) GetCronJobs(ctx context.Context, tenantID string) ([]*internal.CronJob, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, tenant_id, type, payload, cron_expr, priority, max_retries, enabled, created_at FROM cron_jobs WHERE tenant_id=? AND enabled=1`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*internal.CronJob
	for rows.Next() {
		cj := &internal.CronJob{}
		var payload, createdAt string
		var enabled int
		if err := rows.Scan(&cj.ID, &cj.TenantID, &cj.Type, &payload, &cj.CronExpr, &cj.Priority, &cj.MaxRetries, &enabled, &createdAt); err != nil {
			return nil, err
		}
		cj.Payload = []byte(payload)
		cj.Enabled = enabled == 1
		parsed, _ := time.Parse(time.RFC3339Nano, createdAt)
		cj.CreatedAt = parsed
		jobs = append(jobs, cj)
	}
	return jobs, nil
}

func (d *DB) CreateCronJob(ctx context.Context, cj *internal.CronJob) error {
	_, err := d.ExecContext(ctx, `INSERT INTO cron_jobs (id, tenant_id, type, payload, cron_expr, priority, max_retries, enabled, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		cj.ID, cj.TenantID, cj.Type, string(cj.Payload), cj.CronExpr, cj.Priority, cj.MaxRetries, boolInt(cj.Enabled), cj.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (d *DB) DeleteCronJob(ctx context.Context, id string) error {
	_, err := d.ExecContext(ctx, `DELETE FROM cron_jobs WHERE id=?`, id)
	return err
}

func (d *DB) AddDLQ(ctx context.Context, entry *internal.DLQEntry) error {
	_, err := d.ExecContext(ctx, `INSERT INTO dlq (id, job_id, tenant_id, reason, failed_at) VALUES (?,?,?,?,?)`,
		entry.ID, entry.JobID, entry.TenantID, entry.Reason, entry.FailedAt.Format(time.RFC3339Nano))
	return err
}

func (d *DB) ListDLQ(ctx context.Context, tenantID string) ([]*internal.DLQEntry, error) {
	rows, err := d.QueryContext(ctx, `SELECT id, job_id, tenant_id, reason, failed_at, replayed_at FROM dlq WHERE tenant_id=? ORDER BY failed_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []*internal.DLQEntry
	for rows.Next() {
		e, err := scanDLQEntry(rows.Scan)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (d *DB) GetDLQEntry(ctx context.Context, id string) (*internal.DLQEntry, error) {
	row := d.QueryRowContext(ctx, `SELECT id, job_id, tenant_id, reason, failed_at, replayed_at FROM dlq WHERE id=?`, id)
	return scanDLQEntry(row.Scan)
}

func scanDLQEntry(scan func(dest ...interface{}) error) (*internal.DLQEntry, error) {
	e := &internal.DLQEntry{}
	var failedAt, replayedAt *string
	if err := scan(&e.ID, &e.JobID, &e.TenantID, &e.Reason, &failedAt, &replayedAt); err != nil {
		return nil, err
	}
	if failedAt != nil { t, _ := time.Parse(time.RFC3339Nano, *failedAt); e.FailedAt = t }
	if replayedAt != nil { t, _ := time.Parse(time.RFC3339Nano, *replayedAt); e.ReplayedAt = &t }
	return e, nil
}

func (d *DB) MarkDLQReplayed(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE dlq SET replayed_at=? WHERE id=?`, now, id)
	return err
}

func scanJob(scan func(dest ...interface{}) error) (*internal.Job, error) {
	j := &internal.Job{}
	var payload string
	var dagID, dagRunID, idempKey, lastErr, schedAt, startedAt, completedAt, createdAt *string

	err := scan(&j.ID, &j.TenantID, &j.Type, &payload, &j.Priority, &j.Status,
		&dagID, &dagRunID, &idempKey, &j.RetryCount, &j.MaxRetries,
		&lastErr, &schedAt, &startedAt, &completedAt, &createdAt)
	if err != nil {
		return nil, err
	}

	j.Payload = []byte(payload)
	if dagID != nil { j.DAGID = dagID }
	if dagRunID != nil { j.DAGRunID = dagRunID }
	if idempKey != nil { j.IdempotencyKey = idempKey }
	if lastErr != nil { j.LastError = lastErr }
	if schedAt != nil { t, _ := time.Parse(time.RFC3339Nano, *schedAt); j.ScheduledAt = &t }
	if startedAt != nil { t, _ := time.Parse(time.RFC3339Nano, *startedAt); j.StartedAt = &t }
	if completedAt != nil { t, _ := time.Parse(time.RFC3339Nano, *completedAt); j.CompletedAt = &t }
	if createdAt != nil { t, _ := time.Parse(time.RFC3339Nano, *createdAt); j.CreatedAt = t }
	return j, nil
}

// Queue operations

func (d *DB) EnqueueJob(ctx context.Context, job *internal.Job) error {
	payload := string(job.Payload)
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `
		INSERT INTO jobs (id, tenant_id, type, payload, priority, status, dag_id, dag_run_id,
		                  idempotency_key, retry_count, max_retries, last_error, scheduled_at, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		job.ID, job.TenantID, job.Type, payload, job.Priority, job.Status,
		nullStr(job.DAGID), nullStr(job.DAGRunID), nullStr(job.IdempotencyKey),
		job.RetryCount, job.MaxRetries, nullStr(job.LastError),
		formatTime(job.ScheduledAt), job.CreatedAt.Format(time.RFC3339Nano), now)
	return err
}

func (d *DB) DequeueJob(ctx context.Context, jobType string) (*internal.Job, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `
		SELECT id, tenant_id, type, payload, priority, status, dag_id, dag_run_id,
		       idempotency_key, retry_count, max_retries, last_error, scheduled_at,
		       started_at, completed_at, created_at
		FROM jobs
		WHERE status='queued' AND type=? AND (scheduled_at IS NULL OR scheduled_at <= ?)
		  AND tenant_id NOT IN (SELECT tenant_id FROM tenants WHERE paused=1)
		ORDER BY priority DESC, created_at ASC
		LIMIT 1`, jobType, time.Now().Format(time.RFC3339Nano))

	job, err := scanJob(row.Scan)
	if err != nil {
		return nil, fmt.Errorf("no job available: %w", err)
	}

	now := time.Now().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status='running', started_at=?, updated_at=? WHERE id=?`,
		now, now, job.ID)
	if err != nil {
		return nil, err
	}

	job.Status = internal.JobStatusRunning
	return job, tx.Commit()
}

func (d *DB) AckJob(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='completed', completed_at=?, updated_at=? WHERE id=?`,
		now, now, id)
	return err
}

func (d *DB) NackJob(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='queued', updated_at=? WHERE id=?`, now, id)
	return err
}

func (d *DB) RetryJob(ctx context.Context, id string, lastErr *string, backoffSeconds int) error {
	now := time.Now()
	scheduledAt := now.Add(time.Duration(backoffSeconds) * time.Second)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='scheduled', retry_count=retry_count+1, last_error=?, scheduled_at=?, updated_at=? WHERE id=?`,
		nullStr(lastErr), scheduledAt.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), id)
	return err
}

func (d *DB) MoveToDLQ(ctx context.Context, id string, lastErr *string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='dlq', updated_at=? WHERE id=?`, now, id)
	return err
}

func (d *DB) RequeueJob(ctx context.Context, id string) error {
	now := time.Now().Format(time.RFC3339Nano)
	_, err := d.ExecContext(ctx, `UPDATE jobs SET status='queued', retry_count=0, last_error=NULL, scheduled_at=NULL, updated_at=? WHERE id=?`,
		now, id)
	return err
}

func (d *DB) MoveDelayedJobs(ctx context.Context) ([]string, error) {
	now := time.Now().Format(time.RFC3339Nano)
	rows, err := d.QueryContext(ctx, `SELECT id FROM jobs WHERE status='scheduled' AND scheduled_at IS NOT NULL AND scheduled_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
		d.ExecContext(ctx, `UPDATE jobs SET status='queued', scheduled_at=NULL, updated_at=? WHERE id=?`,
			time.Now().Format(time.RFC3339Nano), id)
	}
	return ids, nil
}

func (d *DB) ClaimStaleJobs(ctx context.Context, maxAge time.Duration) ([]string, error) {
	cutoff := time.Now().Add(-maxAge).Format(time.RFC3339Nano)
	rows, err := d.QueryContext(ctx, `SELECT id FROM jobs WHERE status='running' AND started_at IS NOT NULL AND started_at <= ?`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	now := time.Now().Format(time.RFC3339Nano)
	for _, id := range ids {
		d.ExecContext(ctx, `UPDATE jobs SET status='queued', started_at=NULL, updated_at=? WHERE id=?`, now, id)
	}
	return ids, nil
}

func (d *DB) StreamLength(ctx context.Context, jobType string) (int, error) {
	var count int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE status='queued' AND type=?`, jobType).Scan(&count)
	return count, err
}

func (d *DB) CheckIdempotency(ctx context.Context, tenantID, key string) (bool, error) {
	var count int
	err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND idempotency_key=? AND status NOT IN ('cancelled','dlq')`, tenantID, key).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func nullStr(s *string) interface{} {
	if s == nil {
		return nil
	}
	return *s
}

func formatTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.Format(time.RFC3339Nano)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
