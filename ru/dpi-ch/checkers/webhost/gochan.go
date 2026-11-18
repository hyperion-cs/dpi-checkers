package webhost

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/gochan"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/subnetfilter"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/webhostfarm"
)

type checkInput struct {
	group string
	opt   SingleOpt
}

type Item struct {
	Group  string
	Result SingleResult
}

func startChecks(ctx context.Context, in <-chan checkInput, finish func()) <-chan Item {
	return gochan.Start(gochan.GochanOpt[checkInput, Item]{
		Ctx:     ctx,
		Workers: config.Get().Checkers.Webhost.Workers,
		Input:   in,
		Executor: func(in checkInput) Item {
			return Item{Group: in.group, Result: Single(in.opt)}
		},
		Post: finish,
	})
}

type target struct {
	Name           string
	Count          int
	Port           int
	Host           string
	Sni            string
	Tcp1620skip    bool
	RandomHostname bool
}

type RunOut struct {
	Out      <-chan Item
	Progress <-chan string
	Summary  <-chan Summary
}

type Summary struct {
	FarmTimeout bool
	Err         error
}

func Run(ctx context.Context, targets []config.WebhostTarget) RunOut {
	var wg sync.WaitGroup
	var cancel context.CancelFunc
	var runnerErr error
	progressCh := make(chan string, 16)
	summaryCh := make(chan Summary, 1)
	sendProgress(progressCh, "webhost checker => initialization...")

	cfg := config.Get().Checkers.Webhost
	if cfg.FarmTimeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, cfg.FarmTimeout)
	}
	finish := func() {
		wg.Wait()
		summary := Summary{FarmTimeout: errors.Is(ctx.Err(), context.DeadlineExceeded), Err: runnerErr}
		if summary.FarmTimeout && config.Get().Debug {
			log.Println("webhost checker => farming timeout exceeded; stopping...")
		}
		summaryCh <- summary
		close(summaryCh)
		if cancel != nil {
			cancel()
		}
	}

	sf := subnetfilter.New(inetlookup.Default())
	sfGochanIn := make(chan subnetfilter.GochanIn[target])
	sfGochan := subnetfilter.Gochan(subnetfilter.GochanOpt[target]{
		Ctx:          ctx,
		Subnetfilter: sf,
		In:           sfGochanIn,
	})
	sendProgress(progressCh, "subnetfilter => initialized")

	sfItems, err := filterItems(sf, targets)
	if err != nil {
		runnerErr = err
		sendProgress(progressCh, "subnetfilter => internal error (enable debug and check logs)")
		defer finish()
		close(sfGochanIn)
		close(progressCh)
		return RunOut{Out: gochan.Closed[Item](), Progress: progressCh, Summary: summaryCh}
	}
	gochan.Push(ctx, sfGochanIn, sfItems)

	farmGochanIn := make(chan webhostfarm.GochanIn[target])
	farmGochan := webhostfarm.Gochan(webhostfarm.GochanOpt[target]{Ctx: ctx, In: farmGochanIn})
	sendProgress(progressCh, "webhostfarm => initialized")

	wg.Go(func() {
		defer close(farmGochanIn)
		for x := range sfGochan {
			if ctx.Err() != nil {
				continue
			}
			if x.Out.Error != nil {
				runnerErr = errors.Join(runnerErr, x.Out.Error)
				sendProgress(progressCh, fmt.Sprintf("subnetfilter => for %q: %v", x.Bag.Name, x.Out.Error))
				continue
			}
			sendProgress(
				progressCh,
				fmt.Sprintf(`subnetfilter => for "%s" found subnets: %d`, x.Bag.Name, len(x.Out.IpSet.Prefixes())),
			)
			in := webhostfarm.GochanIn[target]{
				Bag: x.Bag,
				In:  webhostfarm.FarmOpt{Subnets: x.Out.IpSet, Count: x.Bag.Count, Port: x.Bag.Port, Sni: x.Bag.Sni},
			}
			select {
			case <-ctx.Done():
			case farmGochanIn <- in:
			}
		}
	})

	checks := make(chan checkInput)
	wg.Go(func() {
		defer close(checks)

		for x := range farmGochan {
			if ctx.Err() != nil {
				continue
			}
			sendProgress(progressCh,
				fmt.Sprintf(`webhostfarm => for "%s" received hosts: %d/%d pcs.`, x.Bag.Name, x.Found, x.Bag.Count),
			)

			in := checkInput{
				group: x.Bag.Name,
				opt: SingleOpt{
					Ctx:            ctx,
					Ip:             x.Out.Ip,
					Port:           x.Out.Port,
					Sni:            x.Bag.Sni,
					Host:           x.Bag.Host,
					Tcp1620skip:    x.Bag.Tcp1620skip,
					RandomHostname: x.Bag.RandomHostname,
				},
			}
			select {
			case <-ctx.Done():
			case checks <- in:
			}
		}
	})
	out := startChecks(ctx, checks, finish)
	sendProgress(progressCh, "webhost checker => initialized")
	go func() {
		wg.Wait()
		close(progressCh)
	}()

	return RunOut{Out: out, Progress: progressCh, Summary: summaryCh}
}

func sendProgress(ch chan<- string, p string) {
	debug := config.Get().Debug
	select {
	case ch <- p:
		if debug {
			log.Println(p)
		}
	default:
	}
}

func filterItems(sf *subnetfilter.Subnetfilter, targets []config.WebhostTarget) ([]subnetfilter.GochanIn[target], error) {
	const logPrefix = "Subnetfilter/CompileFilter"
	items := []subnetfilter.GochanIn[target]{}
	for _, v := range targets {
		f, err := sf.CompileFilter(v.Filter)
		if err != nil {
			log.Println(logPrefix, err)
			return nil, err
		}

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

		items = append(items, subnetfilter.GochanIn[target]{
			Bag: target{
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
	return items, nil
}
