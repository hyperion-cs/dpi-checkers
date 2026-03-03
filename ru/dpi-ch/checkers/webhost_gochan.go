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
	Name           string
	Count          int
	Port           int
	Host           string
	Sni            string
	Tcp1620skip    bool
	RandomHostname bool
}

type WebhostGochanRunnerOut struct {
	Out      <-chan WebhostGochanOut[WebhostGochanBag]
	Progress <-chan string
}

func WebhostGochanRunner(opt WebhostGochanRunnerOpt) WebhostGochanRunnerOut {
	cfg := config.Get().Checkers.Webhost
	progressCh := make(chan string, 16)
	webhostSendProgress(progressCh, "webhost checker => initialization...")

	sf := subnetfilter.New(inetlookup.Default())
	sfGochanIn := make(chan subnetfilter.GochanIn[WebhostGochanBag])
	sfGochan := subnetfilter.Gochan(subnetfilter.GochanOpt[WebhostGochanBag]{
		Ctx:          opt.Ctx,
		Subnetfilter: sf,
		In:           sfGochanIn,
	})
	webhostSendProgress(progressCh, "subnetfilter => initialized")
	gochan.Push(opt.Ctx, sfGochanIn, getSubnetfilterItems(sf, opt.Mode))

	farmGochanIn := make(chan webhostfarm.GochanIn[WebhostGochanBag])
	farmGochan := webhostfarm.Gochan(webhostfarm.GochanOpt[WebhostGochanBag]{Ctx: opt.Ctx, In: farmGochanIn})
	webhostSendProgress(progressCh, "webhostfarm => initialized")

	go func() {
		defer close(farmGochanIn)
		for x := range sfGochan {
			webhostSendProgress(
				progressCh,
				fmt.Sprintf(`subnetfilter => for "%s" found subnets: %d`, x.Bag.Name, len(x.Out.IpSet.Prefixes())),
			)
			in := webhostfarm.GochanIn[WebhostGochanBag]{
				Bag: x.Bag,
				In:  webhostfarm.FarmOpt{Subnets: x.Out.IpSet, Count: x.Bag.Count, Port: x.Bag.Port},
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
	webhostSendProgress(progressCh, "webhost checker => initialized")

	go func() {
		defer close(progressCh)
		defer close(webhostGochanIn)
		defer webhostSendProgress(progressCh, "webhost checker => done")

		for x := range farmGochan {
			webhostSendProgress(progressCh,
				fmt.Sprintf(`webhostfarm => for "%s" found hosts: %d`, x.Bag.Name, len(x.Out)),
			)
			for _, v := range x.Out {
				in := WebhostGochanIn[WebhostGochanBag]{
					Bag: x.Bag,
					In: WebhostSingleOpt{
						Ip:             v.Ip,
						Port:           v.Port,
						Sni:            x.Bag.Sni,
						Host:           x.Bag.Host,
						Tcp1620skip:    x.Bag.Tcp1620skip,
						RandomHostname: x.Bag.RandomHostname,
						KeyLogWriter:   keyLogWriter,
					},
				}
				select {
				case <-opt.Ctx.Done():
					return
				case webhostGochanIn <- in:
				}
			}
		}

	}()

	return WebhostGochanRunnerOut{Out: webhostGochan, Progress: progressCh}
}

func webhostSendProgress(ch chan<- string, p string) {
	select {
	case ch <- p:
	default:
	}
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
		port, count, sni, host := 443, 1, "", ""

		if filterHost, ok := sf.ExtractHostname(f); ok {
			sni = filterHost
			host = filterHost
		}

		if v.Count > 0 {
			count = v.Count
		}
		if v.Port > 0 {
			port = v.Port
		}
		if v.Sni != "" {
			sni = v.Sni
		}
		if v.Host != "" {
			host = v.Host
		}

		items = append(items, subnetfilter.GochanIn[WebhostGochanBag]{
			Bag: WebhostGochanBag{
				Name:           v.Name,
				Count:          count,
				Sni:            sni,
				Host:           host,
				Port:           port,
				RandomHostname: v.RandomHostname,
				Tcp1620skip:    v.Tcp1620skip,
			},
			In: subnetfilter.SubnetfilterIn{Filter: f},
		})
	}
	return items
}
