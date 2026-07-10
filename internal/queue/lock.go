package queue

import (
	"context"
	"sync"
	"time"
)

type lockEntry struct {
	owner    string
	expiresAt time.Time
}

type LockManager struct {
	mu    sync.Mutex
	locks map[string]*lockEntry
	ttl   time.Duration
}

func NewLockManager(ttl time.Duration) *LockManager {
	return &LockManager{
		locks: make(map[string]*lockEntry),
		ttl:   ttl,
	}
}

func (lm *LockManager) Acquire(_ context.Context, jobID, workerID string) (bool, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry, exists := lm.locks[jobID]
	now := time.Now()

	if exists && entry.expiresAt.After(now) && entry.owner != workerID {
		return false, nil
	}

	lm.locks[jobID] = &lockEntry{
		owner:    workerID,
		expiresAt: now.Add(lm.ttl),
	}
	return true, nil
}

func (lm *LockManager) Release(_ context.Context, jobID, workerID string) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry, exists := lm.locks[jobID]
	if !exists || entry.owner != workerID {
		return nil
	}
	delete(lm.locks, jobID)
	return nil
}

func (lm *LockManager) Extend(_ context.Context, jobID, workerID string) (bool, error) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry, exists := lm.locks[jobID]
	if !exists || entry.owner != workerID {
		return false, nil
	}

	entry.expiresAt = time.Now().Add(lm.ttl)
	return true, nil
}

func (lm *LockManager) RefreshLoop(ctx context.Context, jobID, workerID string, interval time.Duration, stop chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			lm.Extend(ctx, jobID, workerID)
		case <-stop:
			return
		case <-ctx.Done():
			return
		}
	}
}
