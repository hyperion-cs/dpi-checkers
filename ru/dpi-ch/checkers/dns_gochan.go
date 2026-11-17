package checkers

import (
	"context"
	"errors"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/gochan"
)

type DnsPlainGochanIn struct {
	Id       string
	Ctx      context.Context
	Provider DnsPlainProvider
	Targets  []DnsTarget
}

type DnsDohGochanIn struct {
	Id          string
	Ctx         context.Context
	DohProvider DnsDohProvider
	Targets     []DnsTarget
}

func DnsPlainGochan(ctx context.Context) <-chan DnsVerdict {
	cfg := config.Get().Checkers.Dns.Resolve
	in := make(chan DnsPlainGochanIn)
	out := gochan.Start(gochan.GochanOpt[DnsPlainGochanIn, DnsVerdict]{
		Ctx:     ctx,
		Workers: cfg.PlainOpt.Workers,
		Input:   in,
		Executor: func(in DnsPlainGochanIn) DnsVerdict {
			if len(in.Provider.Addrs) == 0 {
				return DnsVerdict{
					Provider: in.Id,
					Verdict:  ErrDnsSkip,
				}
			}

			ctx, cancel := context.WithTimeoutCause(in.Ctx, cfg.PlainOpt.ProviderTimeout, ErrDnsProviderTimeout)
			defer cancel()
			matrix := dnsPlainMatrix(ctx, in.Provider, in.Targets)
			hijacking := dnsPlainHijacking(ctx, in.Provider)
			verdict := dnsPlainVerdict(matrix, hijacking)
			if errors.Is(context.Cause(ctx), ErrDnsProviderTimeout) {
				verdict = errors.Join(verdict, ErrDnsProviderTimeout)
			}
			return DnsVerdict{
				Provider: in.Id,
				Verdict:  verdict,
			}
		},
	})

	items := []DnsPlainGochanIn{}
	for _, p := range cfg.Providers {
		items = append(items, DnsPlainGochanIn{
			Id:       p.Name,
			Ctx:      ctx,
			Provider: DnsPlainProvider{Addrs: p.Plain.Hosts, Filter: p.Plain.Filter},
			Targets:  dnsTargets(),
		})
	}

	gochan.Push(ctx, in, items)
	return out
}

func DnsDohGochan(ctx context.Context) <-chan DnsVerdict {
	cfg := config.Get().Checkers.Dns.Resolve
	in := make(chan DnsDohGochanIn)
	out := gochan.Start(gochan.GochanOpt[DnsDohGochanIn, DnsVerdict]{
		Ctx:     ctx,
		Workers: cfg.DohOpt.Workers,
		Input:   in,
		Executor: func(in DnsDohGochanIn) DnsVerdict {
			if len(in.DohProvider.Hosts) == 0 {
				return DnsVerdict{
					Provider: in.Id,
					Verdict:  ErrDnsSkip,
				}
			}

			ctx, cancel := context.WithTimeoutCause(in.Ctx, cfg.DohOpt.ProviderTimeout, ErrDnsProviderTimeout)
			defer cancel()
			matrix := dnsDohMatrix(ctx, in.DohProvider, in.Targets)
			verdict := dnsDohVerdict(matrix)
			if errors.Is(context.Cause(ctx), ErrDnsProviderTimeout) {
				verdict = errors.Join(verdict, ErrDnsProviderTimeout)
			}
			return DnsVerdict{
				Provider: in.Id,
				Verdict:  verdict,
			}
		},
	})

	items := []DnsDohGochanIn{}
	for _, p := range cfg.Providers {
		items = append(items, DnsDohGochanIn{
			Id:          p.Name,
			Ctx:         ctx,
			DohProvider: DnsDohProvider{Hosts: p.DoH.Hosts, Filter: p.DoH.Filter},
			Targets:     dnsTargets(),
		})
	}

	gochan.Push(ctx, in, items)
	return out
}

func DnsLeakGochan(ctx context.Context) <-chan DnsLeakWithIpinfoOut {
	cfg := config.Get().Checkers.Dns.Leak

	in := make(chan struct{})
	out := gochan.Start(gochan.GochanOpt[struct{}, DnsLeakWithIpinfoOut]{
		Ctx:     ctx,
		Workers: cfg.Workers,
		Input:   in,
		Executor: func(in struct{}) DnsLeakWithIpinfoOut {
			return dnsLeakWithIpinfoSingle(ctx)
		},
	})

	gochan.Repeat(ctx, in, struct{}{}, cfg.Times)
	return out
}

func dnsTargets() []DnsTarget {
	cfg := config.Get().Checkers.Dns.Resolve
	targets := []DnsTarget{}
	for _, t := range cfg.Targets {
		targets = append(targets, DnsTarget{Hostname: t.Host, Filter: t.Filter})
	}

	return targets
}
