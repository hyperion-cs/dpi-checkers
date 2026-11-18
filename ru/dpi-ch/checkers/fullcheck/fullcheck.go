package fullcheck

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/cidrwhitelist"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/dns"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/webhost"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/whoami"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type Task uint8

const (
	TaskNone Task = iota
	TaskWhoami
	TaskCidrWhitelist
	TaskDnsLeak
	TaskDnsPlain
	TaskDnsDoh
	TaskWebhost
)

type State uint8

const (
	StateRunning State = iota
	StateOk
	StateError
	StateTimeout
)

type Progress struct {
	Task  Task
	Index int
	State State
	Msg   string
}

func Gochan(ctx context.Context) <-chan Progress {
	cfg := config.Get()
	progressCh := make(chan Progress, 16)
	r := &ReportDto{Version: version}

	go func() {
		defer close(progressCh)
		var wg sync.WaitGroup
		sendProgress(ctx, progressCh, Progress{Msg: "started"})

		var whoamiResult *WhoamiDto
		if slices.Contains(cfg.All.Checkers, "whoami") {
			wg.Go(func() {
				whoamiRes, whoamiErr := whoami.Check(ctx)
				val := whoamiDto(whoamiRes, whoamiErr)
				whoamiResult = &val
				sendProgress(ctx, progressCh, Progress{
					Task: TaskWhoami, State: errorState(whoamiErr), Msg: "whoami ready",
				})
			})
		}

		var cidrResult *CidrWhitelistDto
		if slices.Contains(cfg.All.Checkers, "cidrwhitelist") {
			wg.Go(func() {
				err := cidrwhitelist.Check(ctx)
				val := cidrWhitelistDto(err)
				cidrResult = &val
				state := errorState(err)
				if err == cidrwhitelist.ErrDetected || err == cidrwhitelist.ErrNoInetAccess {
					state = StateOk
				}
				sendProgress(ctx, progressCh, Progress{
					Task: TaskCidrWhitelist, State: state, Msg: "cidrwhitelist ready",
				})
			})
		}

		var dnsLeak *DnsLeakDto
		var dnsPlain *DnsReportDto
		var dnsDoh *DnsReportDto
		if slices.Contains(cfg.All.Checkers, "dns") {
			wg.Go(func() {
				gch := dns.LeakGochan(ctx)
				items := []dns.LeakResult{}
				var leakErr error
				for v := range gch {
					items = append(items, v)
					if v.Err != nil {
						leakErr = v.Err
					}
				}
				val := dnsLeakDto(items)
				dnsLeak = &val
				sendProgress(ctx, progressCh, Progress{
					Task: TaskDnsLeak, State: errorState(leakErr), Msg: "dns:leak ready",
				})
			})

			wg.Go(func() {
				items := []dns.Verdict{}
				gch := dns.PlainGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := dnsReportDto(items)
				dnsPlain = &val
				sendProgress(ctx, progressCh, Progress{
					Task: TaskDnsPlain, State: dnsState(items), Msg: "dns:plain ready",
				})
			})

			wg.Go(func() {
				items := []dns.Verdict{}
				gch := dns.DohGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := dnsReportDto(items)
				dnsDoh = &val
				sendProgress(ctx, progressCh, Progress{
					Task: TaskDnsDoh, State: dnsState(items), Msg: "dns:doh ready",
				})
			})
		}

		var webhostMu sync.Mutex
		webhostSections := map[string]WebhostDto{}
		if slices.Contains(cfg.All.Checkers, "webhost") {
			for i, s := range cfg.Checkers.Webhost.Sections {
				wg.Go(func() {
					gch := webhost.Run(ctx, s.Targets)
					for o := range gch.Out {
						webhostMu.Lock()
						if _, ok := webhostSections[s.Name]; !ok {
							webhostSections[s.Name] = WebhostDto{Items: []WebhostItemDto{}}
						}
						curr := webhostSections[s.Name]
						curr.Items = append(curr.Items, webhostItemDto(o))
						webhostSections[s.Name] = curr
						webhostMu.Unlock()
						sendProgress(ctx, progressCh, Progress{Msg: fmt.Sprintf(`webhost[%s]: "%s" ready`, s.Name, o.Group)})
					}
					summary := <-gch.Summary
					state := errorState(summary.Err)
					if summary.FarmTimeout {
						state = StateTimeout
						webhostMu.Lock()
						curr, ok := webhostSections[s.Name]
						if !ok {
							curr.Items = []WebhostItemDto{}
						}
						curr.FarmTimeout = true
						webhostSections[s.Name] = curr
						webhostMu.Unlock()
					}
					sendProgress(ctx, progressCh, Progress{
						Task: TaskWebhost, Index: i, State: state, Msg: fmt.Sprintf("webhost[%s] ready", s.Name),
					})
				})
			}
		}

		wg.Wait()
		if ctx.Err() != nil {
			return
		}
		r.Whoami = whoamiResult
		r.CidrWhitelist = cidrResult
		r.Webhost = webhostSections
		if dnsLeak != nil || dnsPlain != nil || dnsDoh != nil {
			r.Dns = &DnsDto{
				Leak:  dnsLeak,
				Plain: dnsPlain,
				Doh:   dnsDoh,
			}
		}

		file, err := savePath()
		if err != nil {
			log.Println("fullcheck/savepath:", err)
			sendProgress(ctx, progressCh, Progress{State: StateError, Msg: "error with the output path; enable debug and check the logs"})
			return
		}
		if err := save(file, r); err != nil {
			log.Println("fullcheck/save:", err)
			sendProgress(ctx, progressCh, Progress{State: StateError, Msg: "error when saving to a file; enable debug and check the logs"})
			return
		}
		sendProgress(ctx, progressCh, Progress{State: StateOk, Msg: "done; saved to " + file})
	}()

	return progressCh

}

func dnsState(verdicts []dns.Verdict) State {
	for _, v := range verdicts {
		if errors.Is(v.Verdict, dns.ErrProviderTimeout) {
			return StateTimeout
		}
	}
	return StateOk
}

func errorState(err error) State {
	if err == nil {
		return StateOk
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return StateTimeout
	}
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		return StateTimeout
	}
	return StateError
}

func sendProgress(ctx context.Context, ch chan<- Progress, p Progress) {
	if p.Task != TaskNone || p.State != StateRunning {
		select {
		case ch <- p:
		case <-ctx.Done():
			return
		}
	} else {
		select {
		case ch <- p:
		default:
			return
		}
	}
	if config.Get().Debug {
		log.Println(p)
	}
}
