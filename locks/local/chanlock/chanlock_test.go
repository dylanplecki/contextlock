package chanlock

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChanLock_LockUnlock(t *testing.T) {
	t.Parallel()

	lock := NewChanLock()

	// Attempt without context.
	lock.Lock()
	lock.Unlock() //nolint:staticcheck // Ignore SA2001: empty critical section.

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Attempt with context.
	require.NoError(t, lock.ContextLock(ctx))
	require.NoError(t, lock.ContextUnlock(ctx))
}

func TestChanLock_LockUnlockGoRoutines(t *testing.T) {
	t.Parallel()

	lock := NewChanLock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var sleepEnd time.Time
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		require.NoError(t, lock.ContextLock(ctx))
		// Wait for the second goroutine to start and wait on lock acquisition.
		time.Sleep(500 * time.Millisecond)
		sleepEnd = time.Now()
		// Wait for the internal clock to tick forward a bit before releasing.
		time.Sleep(10 * time.Millisecond)
		require.NoError(t, lock.ContextUnlock(ctx))
	}()

	go func() {
		defer wg.Done()
		// Wait until the first goroutine acquires the lock.
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, lock.ContextLock(ctx))
		assert.True(t, time.Now().After(sleepEnd),
			"second goroutine critical section entered before first goroutine released the lock")
		require.NoError(t, lock.ContextUnlock(ctx))
	}()

	wg.Wait()
}

func TestChanLock_LockAcquireTimeout(t *testing.T) {
	t.Parallel()

	lock := NewChanLock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	require.NoError(t, lock.ContextLock(ctx))

	// This second lock acquisition attempt should timeout on the context.
	require.EqualError(t, lock.ContextLock(ctx), context.DeadlineExceeded.Error())
}

func TestChanLock_LockReleaseTimeout(t *testing.T) {
	t.Parallel()

	lock := NewChanLock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This second lock release attempt should timeout on the context.
	require.EqualError(t, lock.ContextUnlock(ctx), context.DeadlineExceeded.Error())
}

func TestChanLock_NotInitialized(t *testing.T) {
	t.Parallel()

	// Uninitialized channel lock (invalid).
	var lock ChanLock

	// Attempt without context.
	require.Panicsf(t, func() { lock.Lock() }, "Lock should panic when uninitialized")
	require.Panicsf(t, func() { lock.Unlock() }, "Unlock should panic when uninitialized")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Attempt with context.
	require.Error(t, lock.ContextLock(ctx), _chanLockUninitializedError)
	require.Error(t, lock.ContextUnlock(ctx), _chanLockUninitializedError)
}
