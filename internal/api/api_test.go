package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/api"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/db"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/scheduler"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

func setupTest(t *testing.T) (*gin.Engine, *db.DB) {
	t.Helper()
	api.ResetRateLimiterForTest()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(path, 1, zap.NewNop())
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.Close() })
	require.NoError(t, db.RunMigrations(database.DB, zap.NewNop()))

	err = database.CreateTenant(context.Background(), &internal.Tenant{
		ID: "default", Name: "Default", APIKey: "default",
		RateLimits: make(map[string]internal.RateLimit),
		CreatedAt:  time.Now(),
	})
	require.NoError(t, err)

	q := queue.New(database, 1000, zap.NewNop())
	dagResolver := dag.New()
	wsHub := ws.NewHub(zap.NewNop())
	sched := scheduler.New(database, q, zap.NewNop())

	router := api.NewRouter(q, dagResolver, wsHub, sched, database, &internal.Config{}, zap.NewNop())
	return router, database
}

func httpDo(t *testing.T, router *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "default")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestHealth(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "GET", "/health", nil)
	assert.Equal(t, 200, w.Code)
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ok", resp["status"])
}

func TestSubmitAndGetJob(t *testing.T) {
	router, _ := setupTest(t)

	body := `{"type":"default","payload":{"task":"test"},"priority":5}`
	w := httpDo(t, router, "POST", "/api/v1/jobs", []byte(body))
	assert.Equal(t, 201, w.Code)

	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "queued", created.Status)
	assert.NotEmpty(t, created.ID)

	w = httpDo(t, router, "GET", "/api/v1/jobs/"+created.ID+"?type=default", nil)
	assert.Equal(t, 200, w.Code)
	var got internal.Job
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, created.ID, got.ID)
}

func TestSubmitJob_InvalidBody(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "POST", "/api/v1/jobs", []byte(`not json`))
	assert.Equal(t, 400, w.Code)
}

func TestSubmitJob_Scheduled(t *testing.T) {
	router, _ := setupTest(t)
	future := time.Now().Add(time.Hour).Format(time.RFC3339)
	body := `{"type":"default","payload":{},"scheduled_at":"` + future + `"}`
	w := httpDo(t, router, "POST", "/api/v1/jobs", []byte(body))
	assert.Equal(t, 201, w.Code)

	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "scheduled", resp.Status)
}

func TestSubmitJob_Idempotency(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"type":"default","payload":{},"idempotency_key":"unique-1"}`

	w := httpDo(t, router, "POST", "/api/v1/jobs", []byte(body))
	assert.Equal(t, 201, w.Code)

	w = httpDo(t, router, "POST", "/api/v1/jobs", []byte(body))
	assert.Equal(t, 201, w.Code)
}

func TestListJobs(t *testing.T) {
	router, database := setupTest(t)

	for i := 0; i < 3; i++ {
		database.CreateJob(context.Background(), &internal.Job{
			ID: uuid.New().String(), TenantID: "default", Type: "default",
			Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued,
			CreatedAt: time.Now(),
		})
	}

	w := httpDo(t, router, "GET", "/api/v1/jobs", nil)
	assert.Equal(t, 200, w.Code)
	var jobs []internal.Job
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobs))
	assert.Len(t, jobs, 3)
}

func TestGetJob_NotFound(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "GET", "/api/v1/jobs/no-such-id", nil)
	assert.Equal(t, 404, w.Code)
}

func TestCancelJob(t *testing.T) {
	router, database := setupTest(t)
	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusQueued,
		CreatedAt: time.Now(),
	}
	database.CreateJob(context.Background(), job)

	w := httpDo(t, router, "POST", "/api/v1/jobs/"+job.ID+"/cancel", nil)
	assert.Equal(t, 200, w.Code)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "cancelled", resp["status"])
}

func TestCancelJob_NotFound(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "POST", "/api/v1/jobs/no-such-id/cancel", nil)
	assert.Equal(t, 404, w.Code)
}

func TestCreateAndGetDAG(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"name":"test-dag","nodes":[{"id":"a","type":"default","payload":{},"priority":0,"max_retries":3}],"edges":[]}`
	w := httpDo(t, router, "POST", "/api/v1/dags", []byte(body))
	assert.Equal(t, 201, w.Code)

	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	assert.Equal(t, "test-dag", created.Name)
	assert.NotEmpty(t, created.ID)

	w = httpDo(t, router, "GET", "/api/v1/dags/"+created.ID, nil)
	assert.Equal(t, 200, w.Code)
}

func TestCreateDAG_Invalid(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"name":"bad-dag","nodes":[{"id":"a"},{"id":"b"}],"edges":[{"from":"a","to":"c"}]}`
	w := httpDo(t, router, "POST", "/api/v1/dags", []byte(body))
	assert.Equal(t, 400, w.Code)
}

func TestExecuteDAG(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"name":"exec-dag","nodes":[{"id":"a","type":"default","payload":{},"priority":0,"max_retries":3}],"edges":[]}`
	w := httpDo(t, router, "POST", "/api/v1/dags", []byte(body))
	assert.Equal(t, 201, w.Code)

	var dagCreated struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &dagCreated)

	w = httpDo(t, router, "POST", "/api/v1/dags/"+dagCreated.ID+"/execute", nil)
	assert.Equal(t, 201, w.Code)

	var execResp struct {
		DagRunID string   `json:"dag_run_id"`
		Jobs     []string `json:"jobs"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &execResp))
	assert.Len(t, execResp.Jobs, 1)
	assert.NotEmpty(t, execResp.DagRunID)
}

func TestExecuteDAG_NotFound(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "POST", "/api/v1/dags/no-such-dag/execute", nil)
	assert.Equal(t, 404, w.Code)
}

func TestCreateCronJob(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"type":"default","payload":{},"cron_expr":"0 */5 * * * *","priority":0}`
	w := httpDo(t, router, "POST", "/api/v1/cron", []byte(body))
	assert.Equal(t, 201, w.Code)

	var created struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &created)
	assert.NotEmpty(t, created.ID)
}

func TestListAndDeleteCron(t *testing.T) {
	router, _ := setupTest(t)
	body := `{"type":"default","payload":{},"cron_expr":"0 */5 * * * *","priority":0}`
	w := httpDo(t, router, "POST", "/api/v1/cron", []byte(body))
	assert.Equal(t, 201, w.Code)

	var created struct{ ID string }
	json.Unmarshal(w.Body.Bytes(), &created)

	w = httpDo(t, router, "GET", "/api/v1/cron", nil)
	assert.Equal(t, 200, w.Code)
	var jobs []internal.CronJob
	json.Unmarshal(w.Body.Bytes(), &jobs)
	assert.Len(t, jobs, 1)

	w = httpDo(t, router, "DELETE", "/api/v1/cron/"+created.ID, nil)
	assert.Equal(t, 200, w.Code)

	w = httpDo(t, router, "GET", "/api/v1/cron", nil)
	json.Unmarshal(w.Body.Bytes(), &jobs)
	assert.Len(t, jobs, 0)
}

func TestQueueStatus(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "GET", "/api/v1/queue/status", nil)
	assert.Equal(t, 200, w.Code)

	var status map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &status)
	assert.Equal(t, "default", status["tenant_id"])
}

func TestPauseResumeQueue(t *testing.T) {
	router, _ := setupTest(t)

	w := httpDo(t, router, "POST", "/api/v1/queue/pause", nil)
	assert.Equal(t, 200, w.Code)

	w = httpDo(t, router, "POST", "/api/v1/queue/resume", nil)
	assert.Equal(t, 200, w.Code)
}

func TestDLQFlow(t *testing.T) {
	router, database := setupTest(t)

	job := &internal.Job{
		ID: uuid.New().String(), TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusDLQ,
		LastError: strPtr("fail"), CreatedAt: time.Now(),
	}
	database.CreateJob(context.Background(), job)
	database.AddDLQ(context.Background(), &internal.DLQEntry{
		ID: uuid.New().String(), JobID: job.ID, TenantID: "default",
		Reason: "exhausted retries", FailedAt: time.Now(),
	})

	w := httpDo(t, router, "GET", "/api/v1/dlq", nil)
	assert.Equal(t, 200, w.Code)
	var entries []internal.DLQEntry
	json.Unmarshal(w.Body.Bytes(), &entries)
	assert.Len(t, entries, 1)

	w = httpDo(t, router, "POST", "/api/v1/dlq/"+entries[0].ID+"/replay", nil)
	assert.Equal(t, 200, w.Code)
}

func TestAuthMissingKeyDefaults(t *testing.T) {
	router, _ := setupTest(t)
	req := httptest.NewRequest("GET", "/api/v1/queue/status", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)
}

func TestReplayDLQ_NotFound(t *testing.T) {
	router, _ := setupTest(t)
	w := httpDo(t, router, "POST", "/api/v1/dlq/no-such-id/replay", nil)
	assert.Equal(t, 404, w.Code)
}

func strPtr(s string) *string { return &s }
