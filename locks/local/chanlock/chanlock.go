package chanlock

import (
	"context"
	"errors"
	"sync"

	"github.com/dylanplecki/contextlock"
)

const (
	_chanLockUninitializedError = "ChanLock not initialized with NewChanLock"
)

// ChanLock implements a contextlock.ContextLocker that uses a Go channel to
// provide atomic locking and unlocking with context.Context cancellation
// support. This lock is resolved locally and does not require network calls.
type ChanLock struct {
	// Use a sync.Mutex struct to prevent ChanLock struct copying.
	_ sync.Mutex

	channel chan struct{}
}

var _ sync.Locker = (*ChanLock)(nil)
var _ contextlock.ContextLocker = (*ChanLock)(nil)

// NewChanLock creates a new channel-based contextlock.ContextLocker.
func NewChanLock() *ChanLock {
	return &ChanLock{
		channel: make(chan struct{}, 1 /* chan buffer MUST be EXACTLY 1 */),
	}
}

func (cl *ChanLock) checkInit() error {
	if cl.channel == nil {
		return errors.New(_chanLockUninitializedError)
	}
	return nil
}

// Lock implements sync.Locker.
func (cl *ChanLock) Lock() {
	if err := cl.checkInit(); err != nil {
		panic(err)
	}
	cl.channel <- struct{}{}
}

// ContextLock implements contextlock.ContextLocker.
func (cl *ChanLock) ContextLock(ctx context.Context) error {
	if err := cl.checkInit(); err != nil {
		return err
	}
	select {
	case cl.channel <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Unlock implements sync.Locker.
func (cl *ChanLock) Unlock() {
	if err := cl.checkInit(); err != nil {
		panic(err)
	}
	<-cl.channel
}

// ContextUnlock implements contextlock.ContextLocker.
func (cl *ChanLock) ContextUnlock(ctx context.Context) error {
	if err := cl.checkInit(); err != nil {
		return err
	}
	select {
	case <-cl.channel:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
