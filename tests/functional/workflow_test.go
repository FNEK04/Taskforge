package functional_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
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

type apiHelper struct {
	router *gin.Engine
	database *db.DB
}

func newAPI(t *testing.T) *apiHelper {
	t.Helper()
	api.ResetRateLimiterForTest()
	path := filepath.Join(t.TempDir(), "func.db")
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
	return &apiHelper{router: router, database: database}
}

func (h *apiHelper) do(method, path string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "default")
	w := httptest.NewRecorder()
	h.router.ServeHTTP(w, req)
	return w
}

func TestFullJobLifecycle(t *testing.T) {
	app := newAPI(t)

	// 1. Submit a job
	body := `{"type":"default","payload":{"task":"hello"},"priority":5}`
	w := app.do("POST", "/api/v1/jobs", []byte(body))
	require.Equal(t, 201, w.Code)

	var submitResp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &submitResp))
	assert.Equal(t, "queued", submitResp.Status)
	assert.NotEmpty(t, submitResp.ID)
	jobID := submitResp.ID

	// 2. List jobs and verify
	w = app.do("GET", "/api/v1/jobs", nil)
	require.Equal(t, 200, w.Code)
	var jobs []internal.Job
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobs))
	assert.Len(t, jobs, 1)
	assert.Equal(t, jobID, jobs[0].ID)

	// 3. Get job details
	w = app.do("GET", "/api/v1/jobs/"+jobID, nil)
	require.Equal(t, 200, w.Code)
	var job internal.Job
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
	assert.Equal(t, jobID, job.ID)
	assert.Equal(t, `{"task":"hello"}`, string(job.Payload))

	// 4. Cancel the job
	w = app.do("POST", "/api/v1/jobs/"+jobID+"/cancel", nil)
	require.Equal(t, 200, w.Code)
	var cancelResp map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cancelResp))
	assert.Equal(t, "cancelled", cancelResp["status"])

	// 5. Verify status is cancelled
	w = app.do("GET", "/api/v1/jobs/"+jobID, nil)
	require.Equal(t, 200, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
	assert.Equal(t, internal.JobStatusCancelled, job.Status)
}

func TestDAGWorkflow(t *testing.T) {
	app := newAPI(t)

	// 1. Create a DAG with a -> b
	dagBody := `{
		"name": "simple-dag",
		"nodes": [
			{"id":"a","type":"default","payload":{"step":1},"priority":0,"max_retries":3},
			{"id":"b","type":"default","payload":{"step":2},"priority":0,"max_retries":3}
		],
		"edges": [{"from":"a","to":"b"}]
	}`
	w := app.do("POST", "/api/v1/dags", []byte(dagBody))
	require.Equal(t, 201, w.Code)

	var dagResp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dagResp))
	assert.Equal(t, "simple-dag", dagResp.Name)
	dagID := dagResp.ID

	// 2. Get the DAG definition
	w = app.do("GET", "/api/v1/dags/"+dagID, nil)
	require.Equal(t, 200, w.Code)

	// 3. Execute the DAG
	w = app.do("POST", "/api/v1/dags/"+dagID+"/execute", nil)
	require.Equal(t, 201, w.Code)

	var execResp struct {
		DagRunID string   `json:"dag_run_id"`
		Jobs     []string `json:"jobs"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &execResp))
	assert.Len(t, execResp.Jobs, 2)

	// 4. Verify jobs exist and have correct DAG metadata
	for _, jid := range execResp.Jobs {
		w = app.do("GET", "/api/v1/jobs/"+jid, nil)
		require.Equal(t, 200, w.Code)
		var job internal.Job
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &job))
		assert.Equal(t, execResp.DagRunID, *job.DAGRunID)
		assert.NotNil(t, job.DAGID)
		assert.Equal(t, dagID, *job.DAGID)
	}
}

func TestQueuePauseResumeCycle(t *testing.T) {
	app := newAPI(t)

	// 1. Initial status
	w := app.do("GET", "/api/v1/queue/status", nil)
	require.Equal(t, 200, w.Code)
	var status map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, "default", status["tenant_id"])

	// 2. Pause
	w = app.do("POST", "/api/v1/queue/pause", nil)
	require.Equal(t, 200, w.Code)

	w = app.do("GET", "/api/v1/queue/status", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, true, status["paused"])

	// 3. Resume
	w = app.do("POST", "/api/v1/queue/resume", nil)
	require.Equal(t, 200, w.Code)

	w = app.do("GET", "/api/v1/queue/status", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &status))
	assert.Equal(t, false, status["paused"])
}

func TestCronLifecycle(t *testing.T) {
	app := newAPI(t)

	// 1. Create a cron job
	body := `{"type":"default","payload":{},"cron_expr":"0 */5 * * * *","priority":0}`
	w := app.do("POST", "/api/v1/cron", []byte(body))
	require.Equal(t, 201, w.Code)

	var cronResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cronResp))

	// 2. List cron jobs
	w = app.do("GET", "/api/v1/cron", nil)
	require.Equal(t, 200, w.Code)
	var jobs []internal.CronJob
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobs))
	require.Len(t, jobs, 1)
	assert.Equal(t, "0 */5 * * * *", jobs[0].CronExpr)

	// 3. Delete the cron job
	w = app.do("DELETE", "/api/v1/cron/"+cronResp.ID, nil)
	require.Equal(t, 200, w.Code)

	w = app.do("GET", "/api/v1/cron", nil)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobs))
	assert.Len(t, jobs, 0)
}

func TestDLQWorkflow(t *testing.T) {
	app := newAPI(t)

	// 1. Create a job directly in DLQ state
	jID := "dlq-test-job-" + time.Now().Format("150405")
	require.NoError(t, app.database.CreateJob(context.Background(), &internal.Job{
		ID: jID, TenantID: "default", Type: "default",
		Payload: json.RawMessage("{}"), Status: internal.JobStatusDLQ,
		LastError: strPtr("exhausted"), CreatedAt: time.Now(),
	}))
	require.NoError(t, app.database.AddDLQ(context.Background(), &internal.DLQEntry{
		ID: "dlq-entry-1", JobID: jID, TenantID: "default",
		Reason: "max retries exceeded", FailedAt: time.Now(),
	}))

	// 2. List DLQ
	w := app.do("GET", "/api/v1/dlq", nil)
	require.Equal(t, 200, w.Code)
	var entries []internal.DLQEntry
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entries))
	require.Len(t, entries, 1)
	assert.Equal(t, "max retries exceeded", entries[0].Reason)

	// 3. Replay
	w = app.do("POST", "/api/v1/dlq/"+entries[0].ID+"/replay", nil)
	assert.Equal(t, 200, w.Code)
}

func TestHealthEndpoint(t *testing.T) {
	app := newAPI(t)
	w := app.do("GET", "/health", nil)
	assert.Equal(t, 200, w.Code)
	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	assert.Equal(t, "ok", resp["status"])
}

func strPtr(s string) *string { return &s }
