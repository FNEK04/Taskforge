package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/db"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/worker"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

type mockProcessor struct {
	fail   bool
	delay  time.Duration
	callCount int32
}

func (m *mockProcessor) Process(ctx context.Context, job *internal.Job) error {
	atomic.AddInt32(&m.callCount, 1)
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if m.fail {
		return fmt.Errorf("mock failure")
	}
	return nil
}

func setupWorkerTest(t *testing.T) (*worker.Worker, *queue.Queue, *db.DB) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database.DB, zap.NewNop()))

	err = database.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "Default", APIKey: "default",
		CreatedAt: time.Now(),
	})
	require.NoError(t, err)

	q := queue.New(database, 1000, zap.NewNop())
	dagResolver := dag.New()
	wsHub := ws.NewHub(zap.NewNop())
	proc := &mockProcessor{}

	cfg := internal.WorkerConfig{
		Concurrency:       1,
		HeartbeatInterval: time.Second,
		GracefulTimeout:   5 * time.Second,
		PollInterval:      50 * time.Millisecond,
	}

	w := worker.New("test-worker", cfg, q, database, dagResolver, wsHub, proc, zap.NewNop())
	return w, q, database
}

func seedJob(t *testing.T, database *db.DB, status internal.JobStatus) *internal.Job {
	t.Helper()
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: status,
		MaxRetries: 0, CreatedAt: time.Now(),
	}
	require.NoError(t, database.CreateJob(context.Background(), job))
	return job
}

func TestWorker_ProcessesJob(t *testing.T) {
	w, _, database := setupWorkerTest(t)
	seedJob(t, database, internal.JobStatusQueued)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx, []string{"default"}, []string{"default"})

	require.Eventually(t, func() bool {
		job, err := database.GetJob(context.Background(), "non-existent")
		if err != nil {
			// List all jobs instead
			jobs, err := database.ListJobs(context.Background(), "default", "", 100, 0)
			if err != nil || len(jobs) == 0 {
				return false
			}
			return jobs[0].Status == internal.JobStatusCompleted
		}
		return job.Status == internal.JobStatusCompleted
	}, 3*time.Second, 100*time.Millisecond, "job should be completed")

	cancel()
	w.GracefulStop()
	<-w.Stopped()

	jobs, _ := database.ListJobs(context.Background(), "default", "", 100, 0)
	require.Len(t, jobs, 1)
	assert.Equal(t, internal.JobStatusCompleted, jobs[0].Status)
}

func TestWorker_HandlesFailureToDLQ(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	defer database.Close()
	require.NoError(t, db.RunMigrations(database.DB, zap.NewNop()))
	database.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "D", APIKey: "default", CreatedAt: time.Now(),
	})

	q := queue.New(database, 1000, zap.NewNop())
	dagResolver := dag.New()
	wsHub := ws.NewHub(zap.NewNop())
	proc := &mockProcessor{fail: true}

	cfg := internal.WorkerConfig{
		Concurrency:       1,
		HeartbeatInterval: time.Second,
		GracefulTimeout:   5 * time.Second,
		PollInterval:      50 * time.Millisecond,
	}
	w := worker.New("dlq-worker", cfg, q, database, dagResolver, wsHub, proc, zap.NewNop())

	seedJob(t, database, internal.JobStatusQueued)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx, []string{"default"}, []string{"default"})

	require.Eventually(t, func() bool {
		jobs, _ := database.ListJobs(context.Background(), "default", "", 100, 0)
		if len(jobs) == 0 {
			return false
		}
		return jobs[0].Status == internal.JobStatusDLQ
	}, 3*time.Second, 100*time.Millisecond, "job should go to DLQ (max_retries=0)")

	cancel()
	w.GracefulStop()
	<-w.Stopped()
}

func TestWorker_ExportsStoppedChannel(t *testing.T) {
	w, _, _ := setupWorkerTest(t)
	assert.NotNil(t, w.Stopped())
}
