package internal

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTP   HTTPConfig
	DB     DBConfig
	Worker WorkerConfig
}

type HTTPConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type DBConfig struct {
	Path      string
	MaxConns  int
}

type WorkerConfig struct {
	ID               string
	Concurrency      int
	HeartbeatInterval time.Duration
	GracefulTimeout  time.Duration
	PollInterval     time.Duration
}

func LoadConfig() *Config {
	return &Config{
		HTTP: HTTPConfig{
			Port:         getEnv("HTTP_PORT", "8080"),
			ReadTimeout:  getDuration("HTTP_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		},
		DB: DBConfig{
			Path:     getEnv("SQLITE_PATH", "taskforge.db"),
			MaxConns: getInt("DB_MAX_CONNS", 10),
		},
		Worker: WorkerConfig{
			ID:                getEnv("WORKER_ID", "worker-1"),
			Concurrency:       getInt("WORKER_CONCURRENCY", 10),
			HeartbeatInterval: getDuration("WORKER_HEARTBEAT_INTERVAL", 10*time.Second),
			GracefulTimeout:   getDuration("WORKER_GRACEFUL_TIMEOUT", 30*time.Second),
			PollInterval:      getDuration("WORKER_POLL_INTERVAL", 100*time.Millisecond),
		},
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
