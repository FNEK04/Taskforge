package queue

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/taskforge/internal"
	"go.uber.org/zap"
)

type DBIface interface {
	EnqueueJob(ctx context.Context, job *internal.Job) error
	DequeueJob(ctx context.Context, jobType string) (*internal.Job, error)
	AckJob(ctx context.Context, id string) error
	NackJob(ctx context.Context, id string) error
	RetryJob(ctx context.Context, id string, lastErr *string, backoffSeconds int) error
	MoveToDLQ(ctx context.Context, id string, lastErr *string) error
	RequeueJob(ctx context.Context, id string) error
	MoveDelayedJobs(ctx context.Context) ([]string, error)
	ClaimStaleJobs(ctx context.Context, maxAge time.Duration) ([]string, error)
	StreamLength(ctx context.Context, jobType string) (int, error)
	CheckIdempotency(ctx context.Context, tenantID, key string) (bool, error)
	AddDLQ(ctx context.Context, entry *internal.DLQEntry) error
	CreateJob(ctx context.Context, job *internal.Job) error
	CancelJob(ctx context.Context, jobID string) error
	GetJob(ctx context.Context, id string) (*internal.Job, error)
	GetDLQEntry(ctx context.Context, id string) (*internal.DLQEntry, error)
	MarkDLQReplayed(ctx context.Context, id string) error
}

type Queue struct {
	db     DBIface
	lm     *LockManager
	log    *zap.Logger
	notify chan struct{}

	mu       sync.RWMutex
	paused   map[string]bool
	maxLen   int

	stopCh   chan struct{}
}

func New(db DBIface, maxLen int, log *zap.Logger) *Queue {
	return &Queue{
		db:     db,
		lm:     NewLockManager(30 * time.Second),
		log:    log,
		notify: make(chan struct{}, 64),
		paused: make(map[string]bool),
		maxLen: maxLen,
		stopCh: make(chan struct{}),
	}
}

func (q *Queue) LockManager() *LockManager { return q.lm }

func (q *Queue) Enqueue(ctx context.Context, job *internal.Job) error {
	if err := q.checkBackpressure(ctx, job.Type); err != nil {
		return err
	}

	if job.IdempotencyKey != nil {
		exists, err := q.db.CheckIdempotency(ctx, job.TenantID, *job.IdempotencyKey)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
	}

	if job.ScheduledAt != nil && job.ScheduledAt.After(time.Now()) {
		job.Status = internal.JobStatusScheduled
	} else {
		job.Status = internal.JobStatusQueued
	}

	if err := q.db.CreateJob(ctx, job); err != nil {
		return err
	}

	if job.Status == internal.JobStatusQueued {
		q.signal()
	}
	return nil
}

func (q *Queue) Dequeue(ctx context.Context, workerID string, _ string, jobType string, _ time.Duration) (*internal.Job, error) {
	job, err := q.db.DequeueJob(ctx, jobType)
	if err != nil {
		return nil, nil
	}

	acquired, err := q.lm.Acquire(ctx, job.ID, workerID)
	if err != nil || !acquired {
		q.db.NackJob(ctx, job.ID)
		return nil, nil
	}

	return job, nil
}

func (q *Queue) Ack(ctx context.Context, job *internal.Job) error {
	q.lm.Release(ctx, job.ID, job.StreamMsgID)
	return q.db.AckJob(ctx, job.ID)
}

func (q *Queue) Nack(ctx context.Context, job *internal.Job) error {
	q.lm.Release(ctx, job.ID, job.StreamMsgID)
	return q.db.NackJob(ctx, job.ID)
}

func (q *Queue) MoveToDLQ(ctx context.Context, job *internal.Job, reason string) error {
	q.db.MoveToDLQ(ctx, job.ID, &reason)
	dlqEntry := &internal.DLQEntry{
		ID:       uuid.New().String(),
		JobID:    job.ID,
		TenantID: job.TenantID,
		Reason:   reason,
		FailedAt: time.Now(),
	}
	return q.db.AddDLQ(ctx, dlqEntry)
}

func (q *Queue) ReplayFromDLQ(ctx context.Context, tenantID, dlqEntryID string) error {
	entry, err := q.db.GetDLQEntry(ctx, dlqEntryID)
	if err != nil {
		return fmt.Errorf("DLQ entry not found: %w", err)
	}
	if entry.TenantID != tenantID {
		return fmt.Errorf("DLQ entry belongs to a different tenant")
	}
	if entry.ReplayedAt != nil {
		return fmt.Errorf("DLQ entry already replayed")
	}
	if err := q.db.RequeueJob(ctx, entry.JobID); err != nil {
		return fmt.Errorf("failed to requeue job: %w", err)
	}
	if err := q.db.MarkDLQReplayed(ctx, dlqEntryID); err != nil {
		return fmt.Errorf("failed to mark DLQ entry as replayed: %w", err)
	}
	q.signal()
	q.log.Info("replayed DLQ entry", zap.String("entry", dlqEntryID), zap.String("job", entry.JobID))
	return nil
}

func (q *Queue) MoveDelayedJobs(ctx context.Context) ([]string, error) {
	return q.db.MoveDelayedJobs(ctx)
}

func (q *Queue) ClaimStale(ctx context.Context, workerID string, _ time.Duration) (int, error) {
	ids, err := q.db.ClaimStaleJobs(ctx, 60*time.Second)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		q.lm.Release(ctx, id, workerID)
	}
	return len(ids), nil
}

func (q *Queue) Pause(ctx context.Context, tenantID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused[tenantID] = true
	return nil
}

func (q *Queue) Resume(ctx context.Context, tenantID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.paused, tenantID)
	return nil
}

func (q *Queue) IsPaused(ctx context.Context, tenantID string) (bool, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.paused[tenantID], nil
}

func (q *Queue) PendingCount(ctx context.Context, tenantID, jobType string) (int64, error) {
	count, err := q.db.StreamLength(ctx, jobType)
	return int64(count), err
}

func (q *Queue) StreamLen(ctx context.Context, tenantID, jobType string) (int64, error) {
	count, err := q.db.StreamLength(ctx, jobType)
	return int64(count), err
}

func (q *Queue) NotifyChan() <-chan struct{} {
	return q.notify
}

func (q *Queue) signal() {
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

func (q *Queue) checkBackpressure(ctx context.Context, jobType string) error {
	length, err := q.db.StreamLength(ctx, jobType)
	if err != nil {
		return nil
	}
	if length >= q.maxLen {
		return fmt.Errorf("queue full: %d/%d items", length, q.maxLen)
	}
	return nil
}

func (q *Queue) Stop() {
	close(q.stopCh)
}
