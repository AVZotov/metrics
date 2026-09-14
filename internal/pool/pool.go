// Package pool provides a small generic wrapper over sync.Pool that avoids
// the manual type assertion callers otherwise need on Get, and resets
// pooled values automatically on Put.
package pool

import "sync"

// Resetter is implemented by any type that can clear its own state so it's
// safe to hand back out by a Pool. Reset is called automatically on Put.
type Resetter interface {
	Reset()
}

// Pool is a type-safe wrapper around sync.Pool for values of type T.
type Pool[T Resetter] struct {
	pool sync.Pool
}

// New creates a Pool that uses newFunc to construct a fresh T whenever Get
// finds nothing available to reuse.
func New[T Resetter](newFunc func() T) *Pool[T] {
	return &Pool[T]{
		pool: sync.Pool{
			New: func() any {
				return newFunc()
			},
		},
	}
}

// Put resets t and returns it to the pool for reuse. A nil t is ignored.
func (p *Pool[T]) Put(t T) {
	if any(t) == nil {
		return
	}

	t.Reset()
	p.pool.Put(t)
}

// Get returns a T from the pool, creating a new one via New's newFunc if the
// pool is empty.
func (p *Pool[T]) Get() T {
	return p.pool.Get().(T)
}
