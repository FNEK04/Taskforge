package internal

import (
	"encoding/json"
	"time"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusScheduled  JobStatus = "scheduled"
	JobStatusQueued     JobStatus = "queued"
	JobStatusRunning    JobStatus = "running"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
	JobStatusDLQ        JobStatus = "dlq"
)

type Job struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	Status         JobStatus       `json:"status"`
	DAGID          *string         `json:"dag_id,omitempty"`
	DAGRunID       *string         `json:"dag_run_id,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	RetryCount     int             `json:"retry_count"`
	MaxRetries     int             `json:"max_retries"`
	LastError      *string         `json:"last_error,omitempty"`
	ScheduledAt    *time.Time      `json:"scheduled_at,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	Dependencies   []string        `json:"dependencies,omitempty"`
	StreamMsgID    string          `json:"-"`
	StreamKey      string          `json:"-"`
}

type Tenant struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	APIKey     string               `json:"api_key,omitempty"`
	RateLimits map[string]RateLimit `json:"rate_limits"`
	Paused     bool                 `json:"paused"`
	CreatedAt  time.Time            `json:"created_at"`
}

type RateLimit struct {
	MaxPerSecond int `json:"max_per_second"`
	MaxBurst     int `json:"max_burst"`
}

type DAGNode struct {
	ID           string          `json:"id"`
	Type         string          `json:"type"`
	Payload      json.RawMessage `json:"payload"`
	Priority     int             `json:"priority"`
	MaxRetries   int             `json:"max_retries"`
	Dependencies []string        `json:"dependencies,omitempty"`
}

type DAGEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type DAGDefinition struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Name      string    `json:"name"`
	Nodes     []DAGNode `json:"nodes"`
	Edges     []DAGEdge `json:"edges"`
	CreatedAt time.Time `json:"created_at"`
}

type DAGRun struct {
	ID          string               `json:"id"`
	DAGID       string               `json:"dag_id"`
	TenantID    string               `json:"tenant_id"`
	Status      string               `json:"status"`
	JobStatuses map[string]JobStatus `json:"job_statuses"`
	CreatedAt   time.Time            `json:"created_at"`
}

type CronJob struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Type       string          `json:"type"`
	Payload    json.RawMessage `json:"payload"`
	CronExpr   string          `json:"cron_expr"`
	Priority   int             `json:"priority"`
	MaxRetries int             `json:"max_retries"`
	Enabled    bool            `json:"enabled"`
	CreatedAt  time.Time       `json:"created_at"`
}

type DLQEntry struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id"`
	TenantID  string    `json:"tenant_id"`
	Reason    string    `json:"reason"`
	FailedAt  time.Time `json:"failed_at"`
	ReplayedAt *time.Time `json:"replayed_at,omitempty"`
	Job       *Job      `json:"job,omitempty"`
}

type SubmitJobRequest struct {
	Type           string          `json:"type" binding:"required"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	ScheduledAt    *time.Time      `json:"scheduled_at,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	MaxRetries     *int            `json:"max_retries,omitempty"`
	DAGRunID       *string         `json:"dag_run_id,omitempty"`
	Dependencies   []string        `json:"dependencies,omitempty"`
}

type CreateDAGRequest struct {
	Name  string    `json:"name,omitempty"`
	Nodes []DAGNode `json:"nodes" binding:"required"`
	Edges []DAGEdge `json:"edges,omitempty"`
}

type CreateCronRequest struct {
	Type       string          `json:"type" binding:"required"`
	Payload    json.RawMessage `json:"payload"`
	CronExpr   string          `json:"cron_expr" binding:"required"`
	Priority   int             `json:"priority"`
	MaxRetries *int            `json:"max_retries,omitempty"`
}

type WSMessage struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}
