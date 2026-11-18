package dns

import (
	"context"
	"errors"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/gochan"
)

type plainInput struct {
	Id       string
	Ctx      context.Context
	Provider plainProvider
	Targets  []target
}

type dohInput struct {
	Id       string
	Ctx      context.Context
	Provider dohProvider
	Targets  []target
}

func PlainGochan(ctx context.Context) <-chan Verdict {
	cfg := config.Get().Checkers.Dns.Resolve
	in := make(chan plainInput)
	out := gochan.Start(gochan.GochanOpt[plainInput, Verdict]{
		Ctx:     ctx,
		Workers: cfg.PlainOpt.Workers,
		Input:   in,
		Executor: func(in plainInput) Verdict {
			if len(in.Provider.Addrs) == 0 {
				return Verdict{
					Provider: in.Id,
					Verdict:  ErrSkip,
				}
			}

			ctx, cancel := context.WithTimeoutCause(in.Ctx, cfg.PlainOpt.ProviderTimeout, ErrProviderTimeout)
			defer cancel()
			matrix := plainMatrix(ctx, in.Provider, in.Targets)
			hijacking := plainHijacking(ctx, in.Provider)
			verdict := plainVerdict(matrix, hijacking)
			if errors.Is(context.Cause(ctx), ErrProviderTimeout) {
				verdict = errors.Join(verdict, ErrProviderTimeout)
			}
			return Verdict{
				Provider: in.Id,
				Verdict:  verdict,
			}
		},
	})

	items := []plainInput{}
	for _, p := range cfg.Providers {
		items = append(items, plainInput{
			Id:       p.Name,
			Ctx:      ctx,
			Provider: plainProvider{Addrs: p.Plain.Hosts, Filter: p.Plain.Filter},
			Targets:  targets(),
		})
	}

	gochan.Push(ctx, in, items)
	return out
}

func DohGochan(ctx context.Context) <-chan Verdict {
	cfg := config.Get().Checkers.Dns.Resolve
	in := make(chan dohInput)
	out := gochan.Start(gochan.GochanOpt[dohInput, Verdict]{
		Ctx:     ctx,
		Workers: cfg.DohOpt.Workers,
		Input:   in,
		Executor: func(in dohInput) Verdict {
			if len(in.Provider.Hosts) == 0 {
				return Verdict{
					Provider: in.Id,
					Verdict:  ErrSkip,
				}
			}

			ctx, cancel := context.WithTimeoutCause(in.Ctx, cfg.DohOpt.ProviderTimeout, ErrProviderTimeout)
			defer cancel()
			matrix := dohMatrix(ctx, in.Provider, in.Targets)
			verdict := dohVerdict(matrix)
			if errors.Is(context.Cause(ctx), ErrProviderTimeout) {
				verdict = errors.Join(verdict, ErrProviderTimeout)
			}
			return Verdict{
				Provider: in.Id,
				Verdict:  verdict,
			}
		},
	})

	items := []dohInput{}
	for _, p := range cfg.Providers {
		items = append(items, dohInput{
			Id:       p.Name,
			Ctx:      ctx,
			Provider: dohProvider{Hosts: p.DoH.Hosts, Filter: p.DoH.Filter},
			Targets:  targets(),
		})
	}

	gochan.Push(ctx, in, items)
	return out
}

func LeakGochan(ctx context.Context) <-chan LeakResult {
	cfg := config.Get().Checkers.Dns.Leak

	in := make(chan struct{})
	out := gochan.Start(gochan.GochanOpt[struct{}, LeakResult]{
		Ctx:     ctx,
		Workers: cfg.Workers,
		Input:   in,
		Executor: func(in struct{}) LeakResult {
			return leakWithIpInfo(ctx)
		},
	})

	gochan.Repeat(ctx, in, struct{}{}, cfg.Times)
	return out
}

func targets() []target {
	cfg := config.Get().Checkers.Dns.Resolve
	result := []target{}
	for _, t := range cfg.Targets {
		result = append(result, target{Hostname: t.Host, Filter: t.Filter})
	}

	return result
}
