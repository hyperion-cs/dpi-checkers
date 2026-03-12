package subnetfilter

import (
	"context"
	"dpich/config"
	"dpich/gochan"

	"github.com/expr-lang/expr/vm"
	"go4.org/netipx"
)

type SubnetfilterIn struct {
	Filter *vm.Program
}

type SubnetfilterOut struct {
	IpSet *netipx.IPSet
	Error error
}

type GochanIn[T any] struct {
	Bag T
	In  SubnetfilterIn
}

type GochanOut[T any] struct {
	Bag T
	Out SubnetfilterOut
}

type GochanOpt[T any] struct {
	Ctx          context.Context
	Subnetfilter *Subnetfilter
	In           <-chan GochanIn[T]
}

func Gochan[T any](opt GochanOpt[T]) <-chan GochanOut[T] {
	cfg := config.Get().Subnetfilter
	return gochan.Start(gochan.GochanOpt[GochanIn[T], GochanOut[T]]{
		Ctx:     opt.Ctx,
		Workers: cfg.Workers,
		Input:   opt.In,
		Executor: func(in GochanIn[T]) GochanOut[T] {
			ipset, err := opt.Subnetfilter.RunFilter(in.In.Filter)
			return GochanOut[T]{Bag: in.Bag, Out: SubnetfilterOut{IpSet: ipset, Error: err}}
		},
	})
}
