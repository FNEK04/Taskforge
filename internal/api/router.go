package api

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/scheduler"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

func NewRouter(
	q *queue.Queue,
	dagResolver *dag.Resolver,
	wsHub *ws.Hub,
	sched *scheduler.Scheduler,
	db DBIface,
	config *internal.Config,
	log *zap.Logger,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ErrorMiddleware())

	h := NewHandlers(db, q, dagResolver, wsHub, sched, log)

	apiV1 := r.Group("/api/v1")
	apiV1.Use(AuthMiddleware())
	apiV1.Use(RateLimitMiddleware(100))

	{
		apiV1.POST("/jobs", h.SubmitJob)
		apiV1.GET("/jobs", h.ListJobs)
		apiV1.GET("/jobs/:id", h.GetJob)
		apiV1.POST("/jobs/:id/cancel", h.CancelJob)
		apiV1.POST("/jobs/:id/retry", h.RetryJob)
	}

	{
		apiV1.GET("/queue/status", h.QueueStatus)
		apiV1.POST("/queue/pause", h.PauseQueue)
		apiV1.POST("/queue/resume", h.ResumeQueue)
	}

	{
		apiV1.GET("/dlq", h.ListDLQ)
		apiV1.POST("/dlq/:id/replay", h.ReplayDLQ)
	}

	{
		apiV1.POST("/dags", h.CreateDAG)
		apiV1.GET("/dags", h.ListDAGs)
		apiV1.GET("/dags/:id", h.GetDAG)
		apiV1.POST("/dags/:id/execute", h.ExecuteDAG)
	}

	{
		apiV1.POST("/cron", h.CreateCronJob)
		apiV1.GET("/cron", h.ListCronJobs)
		apiV1.DELETE("/cron/:id", h.DeleteCronJob)
	}

	{
		apiV1.GET("/ws", h.WebSocket)
	}

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	distDir := "./frontend/dist"
	if _, err := os.Stat(distDir); err == nil {
		r.StaticFS("/assets", gin.Dir(filepath.Join(distDir, "assets"), false))
		r.StaticFile("/favicon.png", filepath.Join(distDir, "favicon.png"))

		r.NoRoute(func(c *gin.Context) {
			if c.Request.Method == "GET" {
				c.File(filepath.Join(distDir, "index.html"))
				return
			}
			c.Status(http.StatusNotFound)
		})
	}

	return r
}
