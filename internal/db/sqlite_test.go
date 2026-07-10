package db_test

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
	"go.uber.org/zap"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	err = db.RunMigrations(database.DB, zap.NewNop())
	require.NoError(t, err)
	return database
}

func seedTenant(t *testing.T, d *db.DB) {
	t.Helper()
	err := d.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "Default", APIKey: "default",
		RateLimits: make(map[string]internal.RateLimit),
		CreatedAt:  time.Now(),
	})
	require.NoError(t, err)
}

func TestCreateAndGetTenant(t *testing.T) {
	d := newTestDB(t)
	tenant := &internal.Tenant{
		ID: "test-tenant", Name: "Test", APIKey: "test-key",
		RateLimits: make(map[string]internal.RateLimit),
		CreatedAt:  time.Now(),
	}
	err := d.CreateTenant(context.Background(), tenant)
	require.NoError(t, err)

	got, err := d.GetTenant(context.Background(), "test-key")
	require.NoError(t, err)
	assert.Equal(t, "test-tenant", got.ID)
	assert.Equal(t, "Test", got.Name)
}

func TestCreateAndGetJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage(`{"key":"val"}`), Priority: 5,
		Status: internal.JobStatusQueued, MaxRetries: 3, CreatedAt: time.Now(),
	}
	err := d.CreateJob(context.Background(), job)
	require.NoError(t, err)

	got, err := d.GetJob(context.Background(), job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
	assert.JSONEq(t, `{"key":"val"}`, string(got.Payload))
}

func TestGetJob_NotFound(t *testing.T) {
	d := newTestDB(t)
	_, err := d.GetJob(context.Background(), "no-such-id")
	assert.Error(t, err)
}

func TestListJobs_ByStatus(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	for i := 0; i < 3; i++ {
		j := &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "default",
			Payload: json.RawMessage("{}"), Priority: 0,
			Status: internal.JobStatusQueued, MaxRetries: 3, CreatedAt: time.Now(),
		}
		require.NoError(t, d.CreateJob(context.Background(), j))
	}
	run := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0,
		Status: internal.JobStatusRunning, MaxRetries: 3, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), run))

	jobs, err := d.ListJobs(context.Background(), "default", internal.JobStatusQueued, 100, 0)
	require.NoError(t, err)
	assert.Len(t, jobs, 3)

	running, err := d.ListJobs(context.Background(), "default", internal.JobStatusRunning, 100, 0)
	require.NoError(t, err)
	assert.Len(t, running, 1)
}

func TestListJobs_AllStatuses(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	for i := 0; i < 5; i++ {
		j := &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "default",
			Payload: json.RawMessage("{}"), Priority: 0,
			Status: internal.JobStatusQueued, MaxRetries: 3, CreatedAt: time.Now(),
		}
		require.NoError(t, d.CreateJob(context.Background(), j))
	}

	jobs, err := d.ListJobs(context.Background(), "default", "", 100, 0)
	require.NoError(t, err)
	assert.Len(t, jobs, 5)
}

func TestUpdateJobStatus(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := d.UpdateJobStatus(context.Background(), job.ID, internal.JobStatusRunning, nil)
	require.NoError(t, err)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusRunning, got.Status)
}

func TestJobLifecycle(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, MaxRetries: 3,
		CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	require.NoError(t, d.UpdateJobStarted(context.Background(), job.ID))
	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusRunning, got.Status)
	assert.NotNil(t, got.StartedAt)

	require.NoError(t, d.UpdateJobCompleted(context.Background(), job.ID))
	got, _ = d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusCompleted, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestCancelJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := d.CancelJob(context.Background(), job.ID)
	require.NoError(t, err)

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusCancelled, got.Status)
}

func TestCancelJob_AlreadyCompleted(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusCompleted, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	err := d.CancelJob(context.Background(), job.ID)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "cannot be cancelled")
}

func TestEnqueueAndDequeue(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Priority: 0,
		Status: internal.JobStatusQueued, MaxRetries: 3, CreatedAt: time.Now(),
	}
	require.NoError(t, d.EnqueueJob(context.Background(), job))

	got, err := d.DequeueJob(context.Background(), "default")
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, internal.JobStatusRunning, got.Status)
}

func TestDequeue_Empty(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	_, err := d.DequeueJob(context.Background(), "default")
	assert.ErrorContains(t, err, "no job available")
}

func TestAckJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	require.NoError(t, d.AckJob(context.Background(), job.ID))
	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusCompleted, got.Status)
}

func TestNackJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	require.NoError(t, d.NackJob(context.Background(), job.ID))
	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
}

func TestRetryJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning,
		RetryCount: 1, MaxRetries: 3, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	errMsg := "something went wrong"
	require.NoError(t, d.RetryJob(context.Background(), job.ID, &errMsg, 5))

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusScheduled, got.Status)
	assert.Equal(t, 2, got.RetryCount)
	assert.NotNil(t, got.LastError)
	assert.Equal(t, "something went wrong", *got.LastError)
	assert.NotNil(t, got.ScheduledAt)
	assert.True(t, got.ScheduledAt.After(time.Now().Add(-time.Second)))
}

func TestMoveToDLQ(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	errMsg := "exhausted retries"
	require.NoError(t, d.MoveToDLQ(context.Background(), job.ID, &errMsg))

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusDLQ, got.Status)
}

func TestListDLQ(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusDLQ, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	entry := &internal.DLQEntry{
		ID: uuid.New().String(), JobID: job.ID,
		TenantID: "default", Reason: "failed", FailedAt: time.Now(),
	}
	require.NoError(t, d.AddDLQ(context.Background(), entry))

	entries, err := d.ListDLQ(context.Background(), "default")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
	assert.Equal(t, "failed", entries[0].Reason)
}

func TestMoveDelayedJobs(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	past := time.Now().Add(-time.Hour)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusScheduled,
		ScheduledAt: &past, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	ids, err := d.MoveDelayedJobs(context.Background())
	require.NoError(t, err)
	assert.Len(t, ids, 1)
	assert.Equal(t, job.ID, ids[0])

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
}

func TestClaimStaleJobs(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))
	require.NoError(t, d.UpdateJobStarted(context.Background(), job.ID))

	time.Sleep(10 * time.Millisecond)

	ids, err := d.ClaimStaleJobs(context.Background(), 5*time.Millisecond)
	require.NoError(t, err)
	require.Len(t, ids, 1)
	assert.Equal(t, job.ID, ids[0])

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
}

func TestStreamLength(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	for i := 0; i < 3; i++ {
		j := &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "test-type",
			Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued, CreatedAt: time.Now(),
		}
		d.CreateJob(context.Background(), j)
	}

	count, err := d.StreamLength(context.Background(), "test-type")
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestCheckIdempotency(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	key := "my-unique-key"
	exists, err := d.CheckIdempotency(context.Background(), "default", key)
	require.NoError(t, err)
	assert.False(t, exists)

	ik := key
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued,
		IdempotencyKey: &ik, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	exists, err = d.CheckIdempotency(context.Background(), "default", key)
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestDAGCRUD(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	dd := &internal.DAGDefinition{
		ID: uuid.New().String(), TenantID: "default", Name: "test-dag",
		Nodes: []internal.DAGNode{{ID: "a", Type: "default", Payload: json.RawMessage(`{"x":1}`), Priority: 0, MaxRetries: 1}},
		Edges: []internal.DAGEdge{},
		CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateDAGDefinition(context.Background(), dd))

	got, err := d.GetDAGDefinition(context.Background(), dd.ID)
	require.NoError(t, err)
	assert.Equal(t, "test-dag", got.Name)
	assert.Len(t, got.Nodes, 1)
	assert.Equal(t, "a", got.Nodes[0].ID)
}

func TestCronCRUD(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	cj := &internal.CronJob{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), CronExpr: "*/1 * * * *",
		Priority: 0, MaxRetries: 3, Enabled: true, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateCronJob(context.Background(), cj))

	jobs, err := d.GetCronJobs(context.Background(), "default")
	require.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, "*/1 * * * *", jobs[0].CronExpr)

	require.NoError(t, d.DeleteCronJob(context.Background(), cj.ID))
	jobs, err = d.GetCronJobs(context.Background(), "default")
	require.NoError(t, err)
	assert.Len(t, jobs, 0)
}

func TestRequeueJob(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusDLQ,
		RetryCount: 3, LastError: strPtr("fail"), CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))
	require.NoError(t, d.RequeueJob(context.Background(), job.ID))

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusQueued, got.Status)
	assert.Equal(t, 0, got.RetryCount)
	assert.Nil(t, got.LastError)
}

func TestUpdateJobFailed(t *testing.T) {
	d := newTestDB(t)
	seedTenant(t, d)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusRunning,
		RetryCount: 2, MaxRetries: 3, CreatedAt: time.Now(),
	}
	require.NoError(t, d.CreateJob(context.Background(), job))

	errMsg := "process error"
	require.NoError(t, d.UpdateJobFailed(context.Background(), job.ID, 3, &errMsg, internal.JobStatusDLQ))

	got, _ := d.GetJob(context.Background(), job.ID)
	assert.Equal(t, internal.JobStatusDLQ, got.Status)
	assert.Equal(t, 3, got.RetryCount)
	assert.Equal(t, "process error", *got.LastError)
}

func strPtr(s string) *string { return &s }
