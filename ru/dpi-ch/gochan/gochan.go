package gochan

import (
	"context"
	"sync"
)

type GochanOpt[In any, Out any] struct {
	Ctx      context.Context
	Workers  int
	Input    <-chan In
	Executor func(In) Out
	Post     func() // will be executed after all workers finish their tasks
}

func Start[In any, Out any](opt GochanOpt[In, Out]) <-chan Out {
	out := make(chan Out)
	var wg sync.WaitGroup

	for range opt.Workers {
		wg.Go(func() {
			for {
				select {
				case <-opt.Ctx.Done():
					return
				case in, ok := <-opt.Input:
					if !ok {
						return
					}
					select {
					case <-opt.Ctx.Done():
						return
					case out <- opt.Executor(in):
					}
				}
			}
		})
	}

	go func() {
		wg.Wait()
		close(out)
		if opt.Post != nil {
			opt.Post()
		}
	}()

	return out
}

// Run goroutine that push slice into ch, then close it
func Push[In any](ctx context.Context, ch chan<- In, items []In) {
	go func() {
		defer close(ch)
		for _, x := range items {
			select {
			case <-ctx.Done():
				return
			case ch <- x:
			}
		}
	}()
}

// Run goroutine that push the same item into ch n times, then close it.
func Repeat[In any](ctx context.Context, ch chan<- In, item In, n int) {
	go func() {
		defer close(ch)
		for range n {
			select {
			case <-ctx.Done():
				return
			case ch <- item:
			}
		}
	}()
}
