package chanlock

import (
	"context"
	"testing"
	"time"

	"github.com/dylanplecki/contextlock"
	"github.com/dylanplecki/contextlock/internal/contextlocktest"
	"github.com/stretchr/testify/require"
)

func TestChanLock(t *testing.T) {
	t.Parallel()

	contextlocktest.RunContextLockTestSuite(t, context.Background(),
		func(t *testing.T) contextlock.ContextLocker { return NewChanLock() },
	)
}

func TestChanLock_ReleaseContextTimeout(t *testing.T) {
	t.Parallel()

	lock := NewChanLock()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This second lock release attempt should time out on the context.
	require.EqualError(t, lock.UnlockContext(ctx), context.DeadlineExceeded.Error())
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
	require.Error(t, lock.LockContext(ctx), _chanLockUninitializedError)
	require.Error(t, lock.UnlockContext(ctx), _chanLockUninitializedError)
}
