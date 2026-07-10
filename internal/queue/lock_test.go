package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taskforge/internal/queue"
)

func TestLockAcquire_FirstAcquire(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	ok, err := lm.Acquire(context.Background(), "job-1", "worker-1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLockAcquire_SameWorkerReentrant(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	ok, _ := lm.Acquire(context.Background(), "job-1", "worker-1")
	assert.True(t, ok)
	ok, _ = lm.Acquire(context.Background(), "job-1", "worker-1")
	assert.True(t, ok)
}

func TestLockAcquire_DifferentWorkerDenied(t *testing.T) {
	lm := queue.NewLockManager(100 * time.Millisecond)
	ok, _ := lm.Acquire(context.Background(), "job-1", "worker-1")
	assert.True(t, ok)
	ok, _ = lm.Acquire(context.Background(), "job-1", "worker-2")
	assert.False(t, ok)
}

func TestLockAcquire_ExpiredLockAllowsOther(t *testing.T) {
	lm := queue.NewLockManager(50 * time.Millisecond)
	ok, _ := lm.Acquire(context.Background(), "job-1", "worker-1")
	assert.True(t, ok)
	time.Sleep(100 * time.Millisecond)
	ok, _ = lm.Acquire(context.Background(), "job-1", "worker-2")
	assert.True(t, ok)
}

func TestLockRelease_ReleaseOwned(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	lm.Acquire(context.Background(), "job-1", "worker-1")
	err := lm.Release(context.Background(), "job-1", "worker-1")
	assert.NoError(t, err)
	ok, _ := lm.Acquire(context.Background(), "job-1", "worker-2")
	assert.True(t, ok)
}

func TestLockRelease_OtherWorkerNoop(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	lm.Acquire(context.Background(), "job-1", "worker-1")
	err := lm.Release(context.Background(), "job-1", "worker-2")
	assert.NoError(t, err)
	ok, _ := lm.Acquire(context.Background(), "job-1", "worker-2")
	assert.False(t, ok)
}

func TestLockRelease_NonExistentNoop(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	err := lm.Release(context.Background(), "no-such-job", "worker-1")
	assert.NoError(t, err)
}

func TestLockExtend_Owned(t *testing.T) {
	lm := queue.NewLockManager(50 * time.Millisecond)
	lm.Acquire(context.Background(), "job-1", "worker-1")
	time.Sleep(30 * time.Millisecond)
	ok, err := lm.Extend(context.Background(), "job-1", "worker-1")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestLockExtend_NotOwned(t *testing.T) {
	lm := queue.NewLockManager(time.Second)
	ok, err := lm.Extend(context.Background(), "no-such-job", "worker-1")
	require.NoError(t, err)
	assert.False(t, ok)
}
