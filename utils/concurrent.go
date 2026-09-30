package utils

import (
	"fmt"
	"runtime/debug"

	"github.com/GPA-Gruppo-Progetti-Avanzati-SRL/go-core-app"
)

// ErrConcurrentPanic è il codice dell'errore con cui ConcurrentTwo/ConcurrentN riportano un panic
// del task.
const ErrConcurrentPanic = "CONCURRENT-PANIC"

type asyncResult[T any] struct {
	val T
	err *core.Error
}

func runAsync[T any](fn func() (T, *core.Error)) <-chan asyncResult[T] {
	ch := make(chan asyncResult[T], 1)
	go func() { ch <- call(fn) }()
	return ch
}

// call esegue fn trasformando un panic in un core.TechnicalError: in una goroutine lanciata dalla
// libreria un panic non recuperato termina l'intero processo, non la sola chiamata.
func call[T any](fn func() (T, *core.Error)) (r asyncResult[T]) {
	defer func() {
		if p := recover(); p != nil {
			r = asyncResult[T]{err: core.TechnicalError().WithAmbit(core.Ambit).WithCode(ErrConcurrentPanic).
				WithMessage(fmt.Sprintf("panic in concurrent task: %v", p)).
				WithCause(fmt.Errorf("%v\n%s", p, debug.Stack()))}
		}
	}()
	v, e := fn()
	return asyncResult[T]{v, e}
}

// ConcurrentTwo runs two tasks in parallel. Both goroutines always complete — no goroutine leak.
func ConcurrentTwo[A, B any](
	firstTask func() (A, *core.Error),
	secondTask func() (B, *core.Error),
) (A, B, *core.Error) {
	chA, chB := runAsync(firstTask), runAsync(secondTask)
	a, b := <-chA, <-chB
	if a.err != nil {
		return a.val, b.val, a.err
	}
	return a.val, b.val, b.err
}

// ConcurrentN runs fn on each item with at most concurrency goroutines in parallel.
// Results are returned in the same order as inputs; the error of the first failing item (by
// position) is returned. A concurrency <= 0 means no limit: a zero-capacity semaphore made the
// first send block forever, and a negative one made make panic.
// A panic in fn becomes a core.TechnicalError with code CONCURRENT-PANIC instead of killing the process.
func ConcurrentN[T, R any](items []T, concurrency int, fn func(T) (R, *core.Error)) ([]R, *core.Error) {
	if concurrency <= 0 || concurrency > len(items) {
		concurrency = max(len(items), 1)
	}
	chs := make([]<-chan asyncResult[R], len(items))
	sem := make(chan struct{}, concurrency)
	for i, item := range items {
		sem <- struct{}{}
		ch := make(chan asyncResult[R], 1)
		chs[i] = ch
		go func() {
			defer func() { <-sem }()
			ch <- call(func() (R, *core.Error) { return fn(item) })
		}()
	}
	out := make([]R, len(items))
	for i, ch := range chs {
		r := <-ch
		if r.err != nil {
			return nil, r.err
		}
		out[i] = r.val
	}
	return out, nil
}
