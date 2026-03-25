package checkers

import (
	"context"

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
	Id                string
	Ctx               context.Context
	BootstrapProvider DnsPlainProvider
	DohProvider       DnsDohProvider
	Targets           []DnsTarget
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

			matrix := dnsPlainMatrix(in.Ctx, in.Provider, in.Targets)
			return DnsVerdict{
				Provider: in.Id,
				Verdict:  dnsPlainVerdict(matrix),
			}
		},
	})

	items := []DnsPlainGochanIn{}
	for _, p := range cfg.Providers {
		items = append(items, DnsPlainGochanIn{
			Id:       p.Name,
			Ctx:      ctx,
			Provider: DnsPlainProvider{Addrs: p.Plain},
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

			matrix := dnsDohMatrix(in.Ctx, in.BootstrapProvider, in.DohProvider, in.Targets)
			return DnsVerdict{
				Provider: in.Id,
				Verdict:  dnsDohVerdict(matrix),
			}
		},
	})

	items := []DnsDohGochanIn{}
	for _, p := range cfg.Providers {
		items = append(items, DnsDohGochanIn{
			Id:                p.Name,
			Ctx:               ctx,
			BootstrapProvider: DnsPlainProvider{Addrs: p.Plain},
			DohProvider:       DnsDohProvider{Hosts: p.DoH.Hosts, Filter: p.DoH.Filter},
			Targets:           dnsTargets(),
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
			return dnsLeakWithIpinfoSingle()
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
