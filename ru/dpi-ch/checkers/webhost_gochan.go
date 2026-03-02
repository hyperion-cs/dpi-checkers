package checkers

import (
	"context"
	"dpich/config"
	"dpich/gochan"
	"dpich/inetlookup"
	"dpich/subnetfilter"
	"dpich/webhostfarm"
	"fmt"
	"io"
	"os"
)

type WebhostGochanIn[T any] struct {
	Bag T
	In  WebhostSingleOpt
}

type WebhostGochanOut[T any] struct {
	Bag T
	Out WebhostSingleResult
}

type WebhostGochanOpt[T any] struct {
	Ctx  context.Context
	In   <-chan WebhostGochanIn[T]
	Post func()
}

func WebhostGochan[T any](opt WebhostGochanOpt[T]) <-chan WebhostGochanOut[T] {
	cfg := config.Get().Checkers.Webhost
	return gochan.Start(gochan.GochanOpt[WebhostGochanIn[T], WebhostGochanOut[T]]{
		Ctx:     opt.Ctx,
		Workers: cfg.Workers,
		Input:   opt.In,
		Executor: func(in WebhostGochanIn[T]) WebhostGochanOut[T] {
			return WebhostGochanOut[T]{Bag: in.Bag, Out: WebhostSingle(in.In)}
		},
		Post: opt.Post,
	})
}

type WebHostMode int

const (
	WebHostModePopular WebHostMode = iota
	WebHostModeInfra
)

type WebhostGochanRunnerOpt struct {
	Ctx  context.Context
	Mode WebHostMode
}

type WebhostGochanBag struct {
	Name  string
	Count int
}

func WebhostGochanRunner(opt WebhostGochanRunnerOpt) <-chan WebhostGochanOut[WebhostGochanBag] {
	cfg := config.Get().Checkers.Webhost

	sf := subnetfilter.New(inetlookup.Default())
	sfGochanIn := make(chan subnetfilter.GochanIn[WebhostGochanBag])
	sfGochan := subnetfilter.Gochan(subnetfilter.GochanOpt[WebhostGochanBag]{
		Ctx:          opt.Ctx,
		Subnetfilter: sf,
		In:           sfGochanIn,
	})
	gochan.Push(opt.Ctx, sfGochanIn, getSubnetfilterItems(sf, opt.Mode))

	farmGochanIn := make(chan webhostfarm.GochanIn[WebhostGochanBag])
	farmGochan := webhostfarm.Gochan(webhostfarm.GochanOpt[WebhostGochanBag]{Ctx: opt.Ctx, In: farmGochanIn})

	go func() {
		defer close(farmGochanIn)
		for x := range sfGochan {
			fmt.Println("name:", x.Bag.Name, "subnetfilter prefixes:", len(x.Out.IpSet.Prefixes()))
			in := webhostfarm.GochanIn[WebhostGochanBag]{
				Bag: x.Bag,
				In:  webhostfarm.FarmOpt{Subnets: x.Out.IpSet, Count: x.Bag.Count},
			}
			select {
			case <-opt.Ctx.Done():
				return
			case farmGochanIn <- in:
			}
		}
	}()

	var keyLogWriter io.Writer
	var klwPostFunc func()
	if cfg.KeyLogPath != "" {
		file, err := os.OpenFile(cfg.KeyLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		klwPostFunc = func() { file.Close() }
		keyLogWriter = file
	}
	webhostGochanIn := make(chan WebhostGochanIn[WebhostGochanBag])
	webhostGochan := WebhostGochan(WebhostGochanOpt[WebhostGochanBag]{
		Ctx:  opt.Ctx,
		In:   webhostGochanIn,
		Post: klwPostFunc,
	})

	go func() {
		defer close(webhostGochanIn)
		for x := range farmGochan {
			fmt.Println("name:", x.Bag.Name, "farm items:", len(x.Out))
			for _, v := range x.Out {
				in := WebhostGochanIn[WebhostGochanBag]{
					Bag: x.Bag,
					In:  WebhostSingleOpt{Ip: v.Ip, Port: v.Port, KeyLogWriter: keyLogWriter},
				}
				select {
				case <-opt.Ctx.Done():
					return
				case webhostGochanIn <- in:
				}
			}
		}

	}()

	return webhostGochan
}

func getSubnetfilterItems(sf *subnetfilter.Subnetfilter, mode WebHostMode) []subnetfilter.GochanIn[WebhostGochanBag] {
	cfg := config.Get().Checkers.Webhost
	iter := cfg.Infra

	if mode == WebHostModePopular {
		iter = cfg.Popular
	}

	items := []subnetfilter.GochanIn[WebhostGochanBag]{}
	for _, v := range iter {
		// TODO: handle errors
		f, _ := sf.CompileFilter(v.Filter)
		count := 1
		if v.Count != nil {
			count = *v.Count
		}

		items = append(items, subnetfilter.GochanIn[WebhostGochanBag]{
			Bag: WebhostGochanBag{v.Name, count},
			In:  subnetfilter.SubnetfilterIn{Filter: f},
		})
	}
	return items
}
