package contextlocktest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dylanplecki/contextlock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type LockFactory func(t *testing.T) contextlock.ContextLocker

func RunContextLockTestSuite(
	t *testing.T,
	parentContext context.Context,
	lockFactory LockFactory,
	timeoutScaleFactor uint,
) {
	t.Run("lock and unlock", func(t *testing.T) {
		t.Parallel()
		lock := lockFactory(t)

		// Attempt without context.
		lock.Lock()
		lock.Unlock() //nolint:staticcheck // Ignore SA2001: empty critical section.

		ctx, cancel := context.WithTimeout(parentContext, 100*time.Millisecond*time.Duration(timeoutScaleFactor))
		defer cancel()

		// Attempt with context.
		require.NoError(t, lock.LockContext(ctx))
		require.NoError(t, lock.UnlockContext(ctx))
	})

	t.Run("lock and unlock with goroutines", func(t *testing.T) {
		t.Parallel()
		lock := lockFactory(t)

		ctx, cancel := context.WithTimeout(parentContext, 3*time.Second*time.Duration(timeoutScaleFactor))
		defer cancel()

		var sleepEnd time.Time
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			require.NoError(t, lock.LockContext(ctx))
			// Wait for the second goroutine to start and wait on lock acquisition.
			time.Sleep(500 * time.Millisecond * time.Duration(timeoutScaleFactor))
			sleepEnd = time.Now()
			// Wait for the internal clock to tick forward a bit before releasing.
			time.Sleep(10 * time.Millisecond * time.Duration(timeoutScaleFactor))
			require.NoError(t, lock.UnlockContext(ctx))
		}()

		go func() {
			defer wg.Done()
			// Wait until the first goroutine acquires the lock.
			time.Sleep(100 * time.Millisecond * time.Duration(timeoutScaleFactor))
			require.NoError(t, lock.LockContext(ctx))
			assert.True(t, time.Now().After(sleepEnd),
				"second goroutine critical section entered before first goroutine released the lock")
			require.NoError(t, lock.UnlockContext(ctx))
		}()

		wg.Wait()
	})

	t.Run("lock acquisition timeout", func(t *testing.T) {
		t.Parallel()
		lock := lockFactory(t)

		ctx, cancel := context.WithTimeout(parentContext, 100*time.Millisecond*time.Duration(timeoutScaleFactor))
		defer cancel()

		require.NoError(t, lock.LockContext(ctx))
		defer func() { require.NoError(t, lock.UnlockContext(context.Background())) }()

		// This second lock acquisition attempt should time out on the context.
		if err := lock.LockContext(ctx); assert.Error(t, err) {
			assert.Contains(t, err.Error(), context.DeadlineExceeded.Error())
		}
	})
}
