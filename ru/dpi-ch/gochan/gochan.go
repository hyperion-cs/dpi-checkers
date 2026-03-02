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
	Post     func()
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
