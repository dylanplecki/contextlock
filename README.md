Golang Context Lock
===================

[![Go Reference](https://pkg.go.dev/badge/github.com/dylanplecki/contextlock.svg)](https://pkg.go.dev/github.com/dylanplecki/contextlock)
![CI Status](https://github.com/dylanplecki/contextlock/actions/workflows/test.yaml/badge.svg?branch=master)
[![codecov](https://codecov.io/gh/dylanplecki/contextlock/branch/master/graph/badge.svg?token=R66LXRJKJ9)](https://codecov.io/gh/dylanplecki/contextlock)

```golang
// ContextLocker extends the sync.Locker interface to support two new
// lock/unlock methods, LockContext and UnlockContext. These methods perform
// similar operations (and are directly compatible with) the original Lock and
// Unlock methods, but accept an additional context.Context argument used to
// terminate lock acquisition attempts prematurely.
//
// If the lock is not successfully acquired or released via a context-compatible
// method, an error will be returned describing why.
//
// This lock interface can be used to front remote or distributed locks while
// using a similar interface to the local sync.Mutex-style locks. Because of
// this, care should be taken when selecting implementations for and adding
// ContextLocker acquisitions/releases to performance-critical code.
//
// The sync.Locker interface is fully compatible with the new context-compatible
// ContextLocker methods. Thus, it is acceptable for a caller to call Lock then
// UnlockContext, or call LockContext then Unlock.
type ContextLocker interface {
	sync.Locker

	// LockContext provides exclusive control of the ContextLocker object if and
	// only if the error returned is nil. A subsequent call to Unlock or
	// UnlockContext from any goroutine will release control of the
	// ContextLocker object.
	LockContext(ctx context.Context) error

	// UnlockContext releases control of the ContextLocker object. If the
	// ContextLocker has not been locked prior to this call via either Lock o
	// LockContext, the behavior of this method is undefined, but may likely
	// panic.
	UnlockContext(ctx context.Context) error
}
```

## Local Locks

The "local" `ContextLock` types are acquired and released using state local to the executing computer.

These local locks may be slower than a typical mutex, but are typically faster than the distributed locks below.

**Available Local Locks:**

- [`ChanLock`](https://pkg.go.dev/github.com/dylanplecki/contextlock/locks/local/chanlock#ChanLock) - A process-local, context-aware lock using Golang channels.

## Distributed Locks

The "distributed" `ContextLock` types are acquired and released using state present outside the local computer, and may
be located across a network of other computers.

These distributed locks can come in several localities and may have configurable scopes, meaning that locks can be
acquired and released across specific subsets of computers with varying performance implications.

Typically, distributed locks tend to be far slower than local locks.

**Available Local Locks:**

- [`GCSLock`](https://pkg.go.dev/github.com/dylanplecki/contextlock/locks/distributed/gcslock#GCSLock) - A globally-distributed, fault-tolerant lock using Google Cloud Storage.
