package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/taskforge/internal"
	"github.com/taskforge/internal/api"
	"github.com/taskforge/internal/dag"
	"github.com/taskforge/internal/db"
	"github.com/taskforge/internal/queue"
	"github.com/taskforge/internal/scheduler"
	"github.com/taskforge/internal/ws"
	"go.uber.org/zap"
)

func main() {
	log, _ := zap.NewProduction()
	defer log.Sync()

	cfg := internal.LoadConfig()

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
	sched := scheduler.New(sqlDB, q, log)

	router := api.NewRouter(q, dagResolver, wsHub, sched, sqlDB, cfg, log)

	sched.Start()
	if err := sched.LoadAllJobs(context.Background()); err != nil {
		log.Warn("failed to load cron jobs", zap.Error(err))
	}

	srv := &http.Server{
		Addr:         ":" + cfg.HTTP.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
	}

	go func() {
		log.Info("api server starting", zap.String("port", cfg.HTTP.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("server forced to shutdown", zap.Error(err))
	}
	log.Info("server exited")
}
