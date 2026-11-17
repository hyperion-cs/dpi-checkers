package webhostfarm

import (
	"context"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type GochanIn[T any] struct {
	Bag T
	In  FarmOpt
}

type GochanOut[T any] struct {
	Bag   T
	Out   FarmItem
	Found int
}

type GochanOpt[T any] struct {
	Ctx context.Context
	In  <-chan GochanIn[T]
}

func Gochan[T any](opt GochanOpt[T]) <-chan GochanOut[T] {
	cfg := config.Get().WebhostFarm
	out := make(chan GochanOut[T])
	var wg sync.WaitGroup
	for range cfg.Workers {
		wg.Go(func() {
			for {
				select {
				case <-opt.Ctx.Done():
					return
				case in, ok := <-opt.In:
					if !ok {
						return
					}
					found := 0
					// Drain Farm on cancellation so its producer stops before this worker exits.
					for item := range Farm(opt.Ctx, in.In) {
						found++
						select {
						case <-opt.Ctx.Done():
						case out <- GochanOut[T]{Bag: in.Bag, Out: item, Found: found}:
						}
					}
				}
			}
		})
	}
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
