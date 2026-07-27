// Package lock defines a neutral, backend-agnostic distributed-lock primitive.
//
// It is intentionally minimal (two methods) and independent of any scheduler or
// concrete backend, so it can be implemented over Redis, MongoDB, SQL, ... and
// reused for any application need — not only for the batch scheduler.
//
// The go-core backend libraries provide implementations in their own opt-in
// `locker/` subpackages; consumers (e.g. go-core-batch, which adapts a Locker
// to gocron) depend only on this interface.
package lock

import (
	"context"
	"errors"
)

// ErrNotAcquired is returned by Locker.Acquire when the lock is currently held
// by another owner. It signals contention, not an operational failure: callers
// typically skip their critical section and retry later.
var ErrNotAcquired = errors.New("lock: not acquired")

// Locker acquires named distributed locks.
type Locker interface {
	// Acquire attempts to take the lock identified by key. On success it returns
	// a Handle that must be released. It returns ErrNotAcquired when the lock is
	// already held elsewhere, or a non-nil error on backend failure.
	Acquire(ctx context.Context, key string) (Handle, error)
}

// Handle represents a lock that has been acquired.
type Handle interface {
	// Release frees the lock. It is safe to call once per acquired Handle.
	Release(ctx context.Context) error
}
