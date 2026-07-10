package scheduler

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"github.com/taskforge/internal"
	"go.uber.org/zap"
)

type Scheduler struct {
	cron      *cron.Cron
	db        DBIface
	queue     QueueIface
	entries   map[string]cron.EntryID
	mu        sync.RWMutex
	log       *zap.Logger
	ctx       context.Context
	cancel    context.CancelFunc
	delayedCh chan *internal.Job
}

type DBIface interface {
	GetCronJobs(ctx context.Context, tenantID string) ([]*internal.CronJob, error)
	CreateCronJob(ctx context.Context, cj *internal.CronJob) error
	DeleteCronJob(ctx context.Context, id string) error
}

type QueueIface interface {
	Enqueue(ctx context.Context, job *internal.Job) error
}

func New(db DBIface, queue QueueIface, log *zap.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		cron:      cron.New(cron.WithSeconds()),
		db:        db,
		queue:     queue,
		entries:   make(map[string]cron.EntryID),
		log:       log,
		ctx:       ctx,
		cancel:    cancel,
		delayedCh: make(chan *internal.Job, 1000),
	}
}

func (s *Scheduler) Start() {
	s.cron.Start()
	s.log.Info("cron scheduler started")
}

func (s *Scheduler) Stop() {
	s.cancel()
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	s.log.Info("cron scheduler stopped")
}

func (s *Scheduler) AddJob(cj *internal.CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.entries[cj.ID]; exists {
		return nil
	}

	entryID, err := s.cron.AddFunc(cj.CronExpr, func() {
		job := &internal.Job{
			ID:         uuid.New().String(),
			TenantID:   cj.TenantID,
			Type:       cj.Type,
			Payload:    cj.Payload,
			Priority:   cj.Priority,
			MaxRetries: cj.MaxRetries,
			Status:     internal.JobStatusQueued,
			CreatedAt:  time.Now(),
		}
		if err := s.queue.Enqueue(s.ctx, job); err != nil {
			s.log.Error("failed to enqueue cron job",
				zap.String("cron_id", cj.ID),
				zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("invalid cron expr %q: %w", cj.CronExpr, err)
	}

	s.entries[cj.ID] = entryID
	return nil
}

func (s *Scheduler) RemoveJob(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entryID, ok := s.entries[id]; ok {
		s.cron.Remove(entryID)
		delete(s.entries, id)
	}
}

func (s *Scheduler) LoadJobs(ctx context.Context, tenantIDs ...string) error {
	if len(tenantIDs) == 0 {
		return nil
	}
	for _, tenantID := range tenantIDs {
		jobs, err := s.db.GetCronJobs(ctx, tenantID)
		if err != nil {
			s.log.Error("failed to load cron jobs", zap.String("tenant", tenantID), zap.Error(err))
			continue
		}
		for _, cj := range jobs {
			if cj.Enabled {
				if err := s.AddJob(cj); err != nil {
					s.log.Error("failed to add cron job",
						zap.String("cron_id", cj.ID),
						zap.Error(err))
				}
			}
		}
	}
	return nil
}

func (s *Scheduler) CreateCronJob(ctx context.Context, cj *internal.CronJob) error {
	if err := s.db.CreateCronJob(ctx, cj); err != nil {
		return err
	}
	if cj.Enabled {
		return s.AddJob(cj)
	}
	return nil
}

func (s *Scheduler) DeleteCronJob(ctx context.Context, id string) error {
	s.RemoveJob(id)
	return s.db.DeleteCronJob(ctx, id)
}
