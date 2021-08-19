package contextlock

import (
	"context"
	"sync"
)

// ContextLocker extends the sync.Locker interface to support two new
// lock/unlock methods, ContextLock and ContextUnlock. These methods perform
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
// ContextUnlock, or call ContextLock then Unlock.
type ContextLocker interface {
	sync.Locker

	// ContextLock provides exclusive control of the ContextLocker object if and
	// only if the error returned is nil. A subsequent call to Unlock or
	// ContextUnlock from any goroutine will release control of the
	// ContextLocker object.
	ContextLock(ctx context.Context) error

	// ContextUnlock releases control of the ContextLocker object. If the
	// ContextLocker has not been locked prior to this call via either Lock o
	// ContextLock, the behavior of this method is undefined, but may likely
	// panic.
	ContextUnlock(ctx context.Context) error
}
