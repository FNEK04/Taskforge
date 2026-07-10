package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taskforge/internal"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/db"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/worker"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

type echoProcessor struct {
	log *zap.Logger
}

func (p *echoProcessor) Process(ctx context.Context, job *internal.Job) error {
	p.log.Info("processing job", zap.String("id", job.ID), zap.String("type", job.Type))
	var payload map[string]interface{}
	if err := json.Unmarshal(job.Payload, &payload); err == nil {
		p.log.Debug("job payload", zap.Any("payload", payload))
	}
	time.Sleep(100 * time.Millisecond)
	return nil
}

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg := internal.LoadConfig()

	workerID := cfg.Worker.ID
	if workerID == "" {
		workerID = os.Getenv("HOSTNAME")
	}
	if workerID == "" {
		workerID = fmt.Sprintf("worker-%d", time.Now().UnixNano())
	}

	sqlDB, err := db.New(cfg.DB.Path, cfg.DB.MaxConns, log)
	if err != nil {
		log.Fatal("failed to connect to sqlite", zap.Error(err))
	}
	defer sqlDB.Close()

	if err := db.RunMigrations(sqlDB.DB, log); err != nil {
		log.Fatal("failed to run migrations", zap.Error(err))
	}

	q := queue.New(sqlDB, 50000, log)
	dagResolver := dag.New()
	wsHub := ws.NewHub(log)

	proc := &echoProcessor{log: log}

	w := worker.New(workerID, cfg.Worker, q, sqlDB, dagResolver, wsHub, proc, log)

	tenants := []string{"default"}
	if t := os.Getenv("WORKER_TENANTS"); t != "" {
		tenants = split(t, ",")
	}

	jobTypes := []string{"default"}
	if jt := os.Getenv("WORKER_JOB_TYPES"); jt != "" {
		jobTypes = split(jt, ",")
	}

	w.Start(context.Background(), tenants, jobTypes)
	log.Info("worker started", zap.String("worker_id", workerID))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("worker shutting down...")
	w.GracefulStop()
	<-w.Stopped()
	log.Info("worker exited")
}

func split(s, sep string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if string(s[i]) == sep {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}
