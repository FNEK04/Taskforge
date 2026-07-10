package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/scheduler"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

type Handlers struct {
	db        DBIface
	queue     *queue.Queue
	dag       *dag.Resolver
	ws        *ws.Hub
	scheduler *scheduler.Scheduler
	log       *zap.Logger
}

type DBIface interface {
	CreateJob(ctx context.Context, job *internal.Job) error
	UpdateJobStatus(ctx context.Context, id string, status internal.JobStatus, lastErr *string) error
	GetJob(ctx context.Context, id string) (*internal.Job, error)
	ListJobs(ctx context.Context, tenantID string, status internal.JobStatus, limit, offset int) ([]*internal.Job, error)
	GetTenant(ctx context.Context, apiKey string) (*internal.Tenant, error)
	CreateTenant(ctx context.Context, t *internal.Tenant) error
	EnsureTenant(ctx context.Context, tenantID string) error
	GetDAGDefinition(ctx context.Context, id string) (*internal.DAGDefinition, error)
	ListDAGDefinitions(ctx context.Context, tenantID string) ([]*internal.DAGDefinition, error)
	CreateDAGDefinition(ctx context.Context, dd *internal.DAGDefinition) error
	CreateDAGRun(ctx context.Context, run *internal.DAGRun) error
	GetCronJobs(ctx context.Context, tenantID string) ([]*internal.CronJob, error)
	CreateCronJob(ctx context.Context, cj *internal.CronJob) error
	DeleteCronJob(ctx context.Context, id string) error
	AddDLQ(ctx context.Context, entry *internal.DLQEntry) error
	ListDLQ(ctx context.Context, tenantID string) ([]*internal.DLQEntry, error)
	MarkDLQReplayed(ctx context.Context, id string) error
	SetTenantPaused(ctx context.Context, tenantID string, paused bool) error
	CancelJob(ctx context.Context, jobID string) error
	RequeueJob(ctx context.Context, jobID string) error
}

func NewHandlers(db DBIface, q *queue.Queue, d *dag.Resolver, wsHub *ws.Hub, sched *scheduler.Scheduler, log *zap.Logger) *Handlers {
	return &Handlers{
		db:        db,
		queue:     q,
		dag:       d,
		ws:        wsHub,
		scheduler: sched,
		log:       log,
	}
}

func (h *Handlers) SubmitJob(c *gin.Context) {
	var req internal.SubmitJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := getTenantID(c)
	if tenantID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tenant_id required"})
		return
	}

	if err := h.db.EnsureTenant(c, tenantID); err != nil {
		h.log.Error("failed to ensure tenant", zap.Error(err))
	}

	maxRetries := 3
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}

	job := &internal.Job{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		Type:           req.Type,
		Payload:        req.Payload,
		Priority:       req.Priority,
		Status:         internal.JobStatusPending,
		MaxRetries:     maxRetries,
		IdempotencyKey: req.IdempotencyKey,
		ScheduledAt:    req.ScheduledAt,
		DAGRunID:       req.DAGRunID,
		CreatedAt:      time.Now(),
	}

	if err := h.queue.Enqueue(c, job); err != nil {
		h.log.Error("failed to enqueue job", zap.Error(err))
		c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	}

	h.ws.BroadcastJobEvent(tenantID, "job.created", job)
	c.JSON(http.StatusCreated, gin.H{"id": job.ID, "status": job.Status})
}

func (h *Handlers) GetJob(c *gin.Context) {
	jobID := c.Param("id")
	if jobID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id required"})
		return
	}

	job, err := h.db.GetJob(c, jobID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *Handlers) ListJobs(c *gin.Context) {
	tenantID := getTenantID(c)
	status := internal.JobStatus(c.DefaultQuery("status", ""))
	limit := 100
	offset := 0

	jobList, err := h.db.ListJobs(c, tenantID, status, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}
	if jobList == nil {
		jobList = []*internal.Job{}
	}
	c.JSON(http.StatusOK, jobList)
}

func (h *Handlers) CancelJob(c *gin.Context) {
	jobID := c.Param("id")
	if err := h.db.CancelJob(c, jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found or already finished"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

func (h *Handlers) RetryJob(c *gin.Context) {
	jobID := c.Param("id")
	if err := h.db.RequeueJob(c, jobID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found or cannot be retried"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "retried"})
}

func (h *Handlers) QueueStatus(c *gin.Context) {
	tenantID := getTenantID(c)
	jobType := c.Query("type")

	paused, _ := h.queue.IsPaused(c, tenantID)
	streamLen, _ := h.queue.StreamLen(c, tenantID, jobType)
	pendingCount, _ := h.queue.PendingCount(c, tenantID, jobType)

	c.JSON(http.StatusOK, gin.H{
		"tenant_id":   tenantID,
		"paused":      paused,
		"stream_len":  streamLen,
		"pending":     pendingCount,
	})
}

func (h *Handlers) PauseQueue(c *gin.Context) {
	tenantID := getTenantID(c)
	if err := h.queue.Pause(c, tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pause queue"})
		return
	}
	h.db.SetTenantPaused(c, tenantID, true)
	c.JSON(http.StatusOK, gin.H{"status": "paused"})
}

func (h *Handlers) ResumeQueue(c *gin.Context) {
	tenantID := getTenantID(c)
	if err := h.queue.Resume(c, tenantID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resume queue"})
		return
	}
	h.db.SetTenantPaused(c, tenantID, false)
	c.JSON(http.StatusOK, gin.H{"status": "resumed"})
}

func (h *Handlers) ListDLQ(c *gin.Context) {
	tenantID := getTenantID(c)
	entries, err := h.db.ListDLQ(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dlq"})
		return
	}
	if entries == nil {
		entries = []*internal.DLQEntry{}
	}
	c.JSON(http.StatusOK, entries)
}

func (h *Handlers) ReplayDLQ(c *gin.Context) {
	tenantID := getTenantID(c)
	entryID := c.Param("id")
	if err := h.queue.ReplayFromDLQ(c, tenantID, entryID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	h.db.MarkDLQReplayed(c, entryID)
	c.JSON(http.StatusOK, gin.H{"status": "replayed"})
}

func (h *Handlers) CreateDAG(c *gin.Context) {
	var req internal.CreateDAGRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := getTenantID(c)

	if err := h.db.EnsureTenant(c, tenantID); err != nil {
		h.log.Error("failed to ensure tenant", zap.Error(err))
	}

	dd := &internal.DAGDefinition{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Name:      req.Name,
		Nodes:     req.Nodes,
		Edges:     req.Edges,
		CreatedAt: time.Now(),
	}

	if dd.Name == "" {
		if len(dd.Nodes) > 0 {
			dd.Name = dd.Nodes[0].Type + "-dag"
		} else {
			dd.Name = "unnamed-dag"
		}
	}

	if len(dd.Edges) == 0 {
		for _, n := range dd.Nodes {
			for _, dep := range n.Dependencies {
				dd.Edges = append(dd.Edges, internal.DAGEdge{From: dep, To: n.ID})
			}
		}
	}

	if err := h.dag.Validate(dd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.db.CreateDAGDefinition(c, dd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dag"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": dd.ID, "name": dd.Name})
}

func (h *Handlers) GetDAG(c *gin.Context) {
	dagID := c.Param("id")
	dd, err := h.db.GetDAGDefinition(c, dagID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dag not found"})
		return
	}
	c.JSON(http.StatusOK, dd)
}

func (h *Handlers) ListDAGs(c *gin.Context) {
	tenantID := getTenantID(c)
	dags, err := h.db.ListDAGDefinitions(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list dags"})
		return
	}
	if dags == nil {
		dags = []*internal.DAGDefinition{}
	}
	c.JSON(http.StatusOK, dags)
}

func (h *Handlers) ExecuteDAG(c *gin.Context) {
	dagID := c.Param("id")
	tenantID := getTenantID(c)

	if err := h.db.EnsureTenant(c, tenantID); err != nil {
		h.log.Error("failed to ensure tenant", zap.Error(err))
	}

	dd, err := h.db.GetDAGDefinition(c, dagID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "dag not found"})
		return
	}

	if err := h.dag.Validate(dd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid dag: " + err.Error()})
		return
	}

	order, err := h.dag.TopologicalSort(dd)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	runID := uuid.New().String()
	run := &internal.DAGRun{
		ID:          runID,
		DAGID:       dagID,
		TenantID:    tenantID,
		Status:      "running",
		JobStatuses: make(map[string]internal.JobStatus),
		CreatedAt:   time.Now(),
	}

	h.db.CreateDAGRun(c, run)

	nodeMap := make(map[string]internal.DAGNode)
	for _, n := range dd.Nodes {
		nodeMap[n.ID] = n
	}

	var createdJobs []string
	for _, nodeID := range order {
		node := nodeMap[nodeID]
		payload := node.Payload
		if payload == nil {
			payload = json.RawMessage("{}")
		}

		job := &internal.Job{
			ID:         uuid.New().String(),
			TenantID:   tenantID,
			Type:       node.Type,
			Payload:    payload,
			Priority:   node.Priority,
			MaxRetries: node.MaxRetries,
			Status:     internal.JobStatusPending,
			DAGID:      &dagID,
			DAGRunID:   &runID,
			CreatedAt:  time.Now(),
		}

		if err := h.queue.Enqueue(c, job); err != nil {
			h.log.Error("failed to enqueue dag job", zap.Error(err))
			continue
		}

		run.JobStatuses[nodeID] = internal.JobStatusQueued
		createdJobs = append(createdJobs, job.ID)
	}

	c.JSON(http.StatusCreated, gin.H{
		"dag_run_id": runID,
		"jobs":       createdJobs,
	})
}

func (h *Handlers) CreateCronJob(c *gin.Context) {
	var req internal.CreateCronRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	tenantID := getTenantID(c)

	if err := h.db.EnsureTenant(c, tenantID); err != nil {
		h.log.Error("failed to ensure tenant", zap.Error(err))
	}

	maxRetries := 3
	if req.MaxRetries != nil {
		maxRetries = *req.MaxRetries
	}

	cj := &internal.CronJob{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		Type:       req.Type,
		Payload:    req.Payload,
		CronExpr:   req.CronExpr,
		Priority:   req.Priority,
		MaxRetries: maxRetries,
		Enabled:    true,
		CreatedAt:  time.Now(),
	}

	if err := h.scheduler.CreateCronJob(c, cj); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": cj.ID})
}

func (h *Handlers) ListCronJobs(c *gin.Context) {
	tenantID := getTenantID(c)
	jobs, err := h.db.GetCronJobs(c, tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list cron jobs"})
		return
	}
	if jobs == nil {
		jobs = []*internal.CronJob{}
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *Handlers) DeleteCronJob(c *gin.Context) {
	id := c.Param("id")
	if err := h.scheduler.DeleteCronJob(c, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "cron job not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handlers) WebSocket(c *gin.Context) {
	h.ws.HandleWS(c.Writer, c.Request)
}
