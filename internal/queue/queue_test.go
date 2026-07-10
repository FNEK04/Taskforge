package queue_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/db"
	"github.com/taskforge/internal/queue"
	"go.uber.org/zap"
)

func newTestQueue(t *testing.T) (*queue.Queue, *db.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	err = db.RunMigrations(database.DB, zap.NewNop())
	require.NoError(t, err)

	_ = database.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "Default", APIKey: "default",
		RateLimits: make(map[string]internal.RateLimit),
		CreatedAt:  time.Now(),
	})
	return queue.New(database, 1000, zap.NewNop()), database
}

func TestEnqueue_SetsStatusQueued(t *testing.T) {
	q, _ := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0,
		MaxRetries: 3, CreatedAt: time.Now(),
	}
	err := q.Enqueue(context.Background(), job)
	require.NoError(t, err)
	assert.Equal(t, internal.JobStatusQueued, job.Status)
}

func TestDequeue_ReturnsJob(t *testing.T) {
	q, d := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0,
		Status: internal.JobStatusQueued, MaxRetries: 3, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	got, err := q.Dequeue(context.Background(), "worker-1", "", "default", 0)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, job.ID, got.ID)
}

func TestDequeue_EmptyQueue(t *testing.T) {
	q, _ := newTestQueue(t)
	got, err := q.Dequeue(context.Background(), "worker-1", "", "default", 0)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestDequeue_DifferentType(t *testing.T) {
	q, d := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "other-type",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	got, err := q.Dequeue(context.Background(), "worker-1", "", "default", 0)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestAck_CompletesJob(t *testing.T) {
	q, d := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning,
		StreamMsgID: "worker-1", CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := q.Ack(context.Background(), job)
	require.NoError(t, err)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusCompleted, got.Status)
}

func TestNack_RequeuesJob(t *testing.T) {
	q, d := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning,
		StreamMsgID: "worker-1", CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := q.Nack(context.Background(), job)
	require.NoError(t, err)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
}

func TestPauseResume(t *testing.T) {
	q, _ := newTestQueue(t)

	paused, _ := q.IsPaused(context.Background(), "default")
	assert.False(t, paused)

	require.NoError(t, q.Pause(context.Background(), "default"))
	paused, _ = q.IsPaused(context.Background(), "default")
	assert.True(t, paused)

	require.NoError(t, q.Resume(context.Background(), "default"))
	paused, _ = q.IsPaused(context.Background(), "default")
	assert.False(t, paused)
}

func TestMoveToDLQ(t *testing.T) {
	q, d := newTestQueue(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning,
		StreamMsgID: "worker-1", CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := q.MoveToDLQ(context.Background(), job, "failed")
	require.NoError(t, err)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusDLQ, got.Status)

	entries, _ := d.ListDLQ(context.Background(), "default")
	assert.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Reason)
}

func TestEnqueue_Idempotency(t *testing.T) {
	q, d := newTestQueue(t)
	key := "dup-key"

	first := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0, MaxRetries: 3,
		IdempotencyKey: &key, CreatedAt: time.Now(),
	}
	err := q.Enqueue(context.Background(), first)
	require.NoError(t, err)

	second := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0, MaxRetries: 3,
		IdempotencyKey: &key, CreatedAt: time.Now(),
	}
	err = q.Enqueue(context.Background(), second)
	require.NoError(t, err)

	jobs, _ := d.ListJobs(context.Background(), "default", "", 100, 0)
	assert.Len(t, jobs, 1)
}

func TestEnqueue_Scheduled(t *testing.T) {
	q, d := newTestQueue(t)
	future := time.Now().Add(time.Hour)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0, MaxRetries: 3,
		ScheduledAt: &future, CreatedAt: time.Now(),
	}
	err := q.Enqueue(context.Background(), job)
	require.NoError(t, err)
	assert.Equal(t, internal.JobStatusScheduled, job.Status)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusScheduled, got.Status)
}

func TestBackpressure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	defer database.Close()
	require.NoError(t, db.RunMigrations(database.DB, zap.NewNop()))
	database.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "D", APIKey: "default", CreatedAt: time.Now(),
	})

	q := queue.New(database, 2, zap.NewNop())
	for i := 0; i < 2; i++ {
		err := q.Enqueue(context.Background(), &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "test-bp",
			Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
		})
		require.NoError(t, err)
	}
	err = q.Enqueue(context.Background(), &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "test-bp",
		Payload: json.RawMessage("{}"), CreatedAt: time.Now(),
	})
	assert.ErrorContains(t, err, "queue full")
}

func TestPendingCount(t *testing.T) {
	q, d := newTestQueue(t)

	for i := 0; i < 3; i++ {
		j := &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "count-me",
			Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
		}
		d.CreateJob(context.Background(), j)
	}

	count, err := q.PendingCount(context.Background(), "default", "count-me")
	require.NoError(t, err)
	assert.Equal(t, int64(3), count)
}

func TestLockManagerAccess(t *testing.T) {
	q, _ := newTestQueue(t)
	lm := q.LockManager()
	assert.NotNil(t, lm)
}
