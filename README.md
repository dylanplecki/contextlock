Golang Context Lock
===================

[![Go Reference](https://pkg.go.dev/badge/github.com/dylanplecki/contextlock.svg)](https://pkg.go.dev/github.com/dylanplecki/contextlock)
![CI Status](https://github.com/dylanplecki/contextlock/actions/workflows/test.yaml/badge.svg?branch=master)
[![codecov](https://codecov.io/gh/dylanplecki/contextlock/branch/master/graph/badge.svg?token=R66LXRJKJ9)](https://codecov.io/gh/dylanplecki/contextlock)

This package exposes a new `ContextLocker` interface (similar to the `sync.Locker` interface) and several
local and distributed `ContextLocker` implementations.

---

## Context Lock Implementations

### Local

The "local" `ContextLock` types are acquired and released using state local to the executing computer.

These local locks may be slower than a typical mutex, but are typically faster than the distributed locks below.

**Available Local Locks**

- `ChanLock` - A process-local, context-aware lock using Golang channels.

### Distributed

The "distributed" `ContextLock` types are acquired and released using state present outside the local computer, and may
be located across a network of other computers.

These distributed locks can come in several localities and may have configurable scopes, meaning that locks can be
acquired and released across specific subsets of computers with varying performance implications.

Typically, distributed locks tend to be far slower than local locks.

**Available Local Locks**

- `GCSLock` - A globally-distributed, fault-tolerant lock using Google Cloud Storage.
