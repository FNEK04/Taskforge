package worker

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

type Processor interface {
	Process(ctx context.Context, job *internal.Job) error
}

type Worker struct {
	id        string
	cfg       internal.WorkerConfig
	queue     *queue.Queue
	db        DBIface
	dag       *dag.Resolver
	ws        *ws.Hub
	processor Processor
	log       *zap.Logger

	wg         sync.WaitGroup
	activeJobs sync.WaitGroup
	shutdown   chan struct{}
	stopped    chan struct{}
	mu         sync.Mutex
	isRunning  bool
}

type DBIface interface {
	UpdateJobStarted(ctx context.Context, id string) error
	UpdateJobCompleted(ctx context.Context, id string) error
	UpdateJobFailed(ctx context.Context, id string, retryCount int, lastErr *string, status internal.JobStatus) error
	RetryJob(ctx context.Context, id string, lastErr *string, backoffSeconds int) error
}

func New(id string, cfg internal.WorkerConfig, q *queue.Queue, db DBIface, dagResolver *dag.Resolver, wsHub *ws.Hub, processor Processor, log *zap.Logger) *Worker {
	return &Worker{
		id:        id,
		cfg:       cfg,
		queue:     q,
		db:        db,
		dag:       dagResolver,
		ws:        wsHub,
		processor: processor,
		log:       log.Named("worker").With(zap.String("worker_id", id)),
		shutdown:  make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

func (w *Worker) Start(ctx context.Context, tenants []string, jobTypes []string) {
	w.mu.Lock()
	if w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = true
	w.mu.Unlock()

	w.log.Info("worker starting",
		zap.Int("concurrency", w.cfg.Concurrency),
		zap.Strings("tenants", tenants),
		zap.Strings("types", jobTypes))

	delayedTicker := time.NewTicker(500 * time.Millisecond)
	claimTicker := time.NewTicker(10 * time.Second)

	for i := 0; i < w.cfg.Concurrency; i++ {
		w.wg.Add(1)
		go w.processLoop(ctx, jobTypes)
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-delayedTicker.C:
				moved, err := w.queue.MoveDelayedJobs(ctx)
				if err != nil {
					w.log.Debug("delayed poll error", zap.Error(err))
				}
				if len(moved) > 0 {
					w.log.Debug("moved delayed jobs", zap.Int("count", len(moved)))
				}
			case <-w.shutdown:
				delayedTicker.Stop()
				claimTicker.Stop()
				return
			}
		}
	}()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-claimTicker.C:
				claimed, err := w.queue.ClaimStale(ctx, w.id, w.cfg.HeartbeatInterval)
				if err != nil {
					w.log.Debug("claim error", zap.Error(err))
				} else if claimed > 0 {
					w.log.Info("claimed stale jobs", zap.Int("count", claimed))
				}
			case <-w.shutdown:
				return
			}
		}
	}()

	go func() {
		<-ctx.Done()
		w.GracefulStop()
	}()

	w.log.Info("worker started")
}

func (w *Worker) GracefulStop() {
	w.mu.Lock()
	if !w.isRunning {
		w.mu.Unlock()
		return
	}
	w.isRunning = false
	w.mu.Unlock()

	w.log.Info("worker shutting down gracefully")
	close(w.shutdown)

	done := make(chan struct{})
	go func() {
		w.activeJobs.Wait()
		close(done)
	}()

	select {
	case <-done:
		w.log.Info("all active jobs completed")
	case <-time.After(w.cfg.GracefulTimeout):
		w.log.Warn("graceful shutdown timeout reached, some jobs may be abandoned")
	}

	w.wg.Wait()
	close(w.stopped)
	w.log.Info("worker stopped")
}

func (w *Worker) Stopped() <-chan struct{} { return w.stopped }

func (w *Worker) processLoop(ctx context.Context, jobTypes []string) {
	defer w.wg.Done()

	for {
		select {
		case <-w.shutdown:
			return
		case <-w.queue.NotifyChan():
		default:
		}

		processed := false
		for _, jobType := range jobTypes {
			paused, _ := w.queue.IsPaused(ctx, "")
			if paused {
				time.Sleep(300 * time.Millisecond)
				continue
			}

			job, err := w.queue.Dequeue(ctx, w.id, "", jobType, 0)
			if err != nil {
				continue
			}
			if job == nil {
				continue
			}

			processed = true
			w.activeJobs.Add(1)
			go func(j *internal.Job) {
				defer w.activeJobs.Done()
				w.processJob(ctx, j)
			}(job)
		}

		if !processed {
			select {
			case <-w.shutdown:
				return
			case <-w.queue.NotifyChan():
			case <-time.After(300 * time.Millisecond):
			}
		}
	}
}

func (w *Worker) processJob(ctx context.Context, job *internal.Job) {
	w.log.Info("processing job", zap.String("id", job.ID), zap.String("type", job.Type))

	acquired, err := w.queue.LockManager().Acquire(ctx, job.ID, w.id)
	if err != nil || !acquired {
		w.log.Warn("failed to acquire lock for job", zap.String("id", job.ID), zap.Error(err))
		return
	}

	lockStop := make(chan struct{})
	go w.queue.LockManager().RefreshLoop(ctx, job.ID, w.id, w.cfg.HeartbeatInterval/2, lockStop)
	defer close(lockStop)
	defer w.queue.LockManager().Release(ctx, job.ID, w.id)

	w.db.UpdateJobStarted(ctx, job.ID)
	w.ws.BroadcastJobEvent(job.TenantID, "job.started", job)

	processErr := w.processor.Process(ctx, job)

	if processErr != nil {
		w.handleFailure(ctx, job, processErr)
	} else {
		w.handleSuccess(ctx, job)
	}
}

func (w *Worker) handleSuccess(ctx context.Context, job *internal.Job) {
	job.Status = internal.JobStatusCompleted
	now := time.Now()
	job.CompletedAt = &now
	w.queue.LockManager().Release(ctx, job.ID, w.id)
	w.db.UpdateJobCompleted(ctx, job.ID)
	w.ws.BroadcastJobEvent(job.TenantID, "job.completed", job)
}

func (w *Worker) handleFailure(ctx context.Context, job *internal.Job, processErr error) {
	errMsg := processErr.Error()
	job.RetryCount++

	if job.RetryCount >= job.MaxRetries {
		job.Status = internal.JobStatusDLQ
		w.db.UpdateJobFailed(ctx, job.ID, job.RetryCount, &errMsg, internal.JobStatusDLQ)
		w.queue.MoveToDLQ(ctx, job, fmt.Sprintf("max retries (%d) exceeded: %s", job.MaxRetries, errMsg))
		w.ws.BroadcastJobEvent(job.TenantID, "job.dlq", job)
		w.log.Warn("job moved to DLQ", zap.String("id", job.ID), zap.Error(processErr))
	} else {
		backoff := int(math.Pow(2, float64(job.RetryCount)))
		if backoff > 60 {
			backoff = 60
		}
		job.Status = internal.JobStatusScheduled
		w.db.RetryJob(ctx, job.ID, &errMsg, backoff)
		w.ws.BroadcastJobEvent(job.TenantID, "job.retry", job)
		w.log.Warn("job will retry",
			zap.String("id", job.ID),
			zap.Int("attempt", job.RetryCount),
			zap.Int("backoff_sec", backoff),
			zap.Error(processErr))
	}
}
