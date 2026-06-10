package core

type asyncResult[T any] struct {
	val T
	err *ApplicationError
}

func runAsync[T any](fn func() (T, *ApplicationError)) <-chan asyncResult[T] {
	ch := make(chan asyncResult[T], 1)
	go func() { v, e := fn(); ch <- asyncResult[T]{v, e} }()
	return ch
}

// ConcurrentTwo runs two tasks in parallel. Both goroutines always complete — no goroutine leak.
func ConcurrentTwo[A, B any](
	firstTask func() (A, *ApplicationError),
	secondTask func() (B, *ApplicationError),
) (A, B, *ApplicationError) {
	chA, chB := runAsync(firstTask), runAsync(secondTask)
	a, b := <-chA, <-chB
	if a.err != nil {
		return a.val, b.val, a.err
	}
	return a.val, b.val, b.err
}

// ConcurrentN runs fn on each item with at most concurrency goroutines in parallel.
// Results are returned in the same order as inputs; the first error encountered is returned.
func ConcurrentN[T, R any](items []T, concurrency int, fn func(T) (R, *ApplicationError)) ([]R, *ApplicationError) {
	chs := make([]<-chan asyncResult[R], len(items))
	sem := make(chan struct{}, concurrency)
	for i, item := range items {
		i, item := i, item
		sem <- struct{}{}
		ch := make(chan asyncResult[R], 1)
		chs[i] = ch
		go func() {
			v, e := fn(item)
			ch <- asyncResult[R]{v, e}
			<-sem
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
