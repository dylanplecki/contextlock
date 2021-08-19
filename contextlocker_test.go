package contextlock_test

import (
	"testing"

	"github.com/dylanplecki/contextlock"
	"github.com/mco-gh/gcslock"
)

func TestContextLock_Compatibility(t *testing.T) {
	t.Parallel()

	// Test ContextLock compatibility with other implementations.
	var _ gcslock.ContextLocker = (contextlock.ContextLocker)(nil)
}
