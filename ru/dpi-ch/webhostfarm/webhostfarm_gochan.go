package webhostfarm

import (
	"context"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/gochan"
)

type GochanIn[T any] struct {
	Bag T
	In  FarmOpt
}

type GochanOut[T any] struct {
	Bag T
	Out []FarmItem
}

type GochanOpt[T any] struct {
	Ctx context.Context
	In  <-chan GochanIn[T]
}

func Gochan[T any](opt GochanOpt[T]) <-chan GochanOut[T] {
	cfg := config.Get().WebhostFarm
	return gochan.Start(gochan.GochanOpt[GochanIn[T], GochanOut[T]]{
		Ctx:     opt.Ctx,
		Workers: cfg.Workers,
		Input:   opt.In,
		Executor: func(in GochanIn[T]) GochanOut[T] {
			return GochanOut[T]{Bag: in.Bag, Out: Farm(in.In)}
		},
	})
}
