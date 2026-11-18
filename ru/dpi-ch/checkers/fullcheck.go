package checkers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"sync"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

type FullCheckTask uint8

const (
	FullCheckTaskNone FullCheckTask = iota
	FullCheckTaskWhoami
	FullCheckTaskCidrWhitelist
	FullCheckTaskDnsLeak
	FullCheckTaskDnsPlain
	FullCheckTaskDnsDoh
	FullCheckTaskWebhost
)

type FullCheckState uint8

const (
	FullCheckStateRunning FullCheckState = iota
	FullCheckStateOk
	FullCheckStateError
	FullCheckStateTimeout
)

type FullCheckProgress struct {
	Task  FullCheckTask
	Index int
	State FullCheckState
	Msg   string
}

func FullCheckGochan(ctx context.Context) <-chan FullCheckProgress {
	cfg := config.Get()
	progressCh := make(chan FullCheckProgress, 16)
	r := &FullCheckDto{Version: fullCheckVersion}

	go func() {
		defer close(progressCh)
		var wg sync.WaitGroup
		fullCheckSendProgress(ctx, progressCh, FullCheckProgress{Msg: "started"})

		var whoami *FullCheckWhoamiDto
		if slices.Contains(cfg.All.Checkers, "whoami") {
			wg.Go(func() {
				whoamiRes, whoamiErr := Whoami(ctx)
				val := fullCheckWhoamiDto(whoamiRes, whoamiErr)
				whoami = &val
				fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
					Task: FullCheckTaskWhoami, State: fullCheckErrorState(whoamiErr), Msg: "whoami ready",
				})
			})
		}

		var cidrwhitelist *FullCheckCidrwhitelistDto
		if slices.Contains(cfg.All.Checkers, "cidrwhitelist") {
			wg.Go(func() {
				err := CidrWhitelist(ctx)
				val := fullCheckCidrwhitelistDto(err)
				cidrwhitelist = &val
				state := fullCheckErrorState(err)
				if err == ErrCidrWhitelistDetected || err == ErrCidrWhitelistNoInetAccess {
					state = FullCheckStateOk
				}
				fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
					Task: FullCheckTaskCidrWhitelist, State: state, Msg: "cidrwhitelist ready",
				})
			})
		}

		var dnsLeak *FullCheckDnsLeakDto
		var dnsPlain *FullCheckDnsReportDto
		var dnsDoh *FullCheckDnsReportDto
		if slices.Contains(cfg.All.Checkers, "dns") {
			wg.Go(func() {
				gch := DnsLeakGochan(ctx)
				items := []DnsLeakWithIpinfoOut{}
				var leakErr error
				for v := range gch {
					items = append(items, v)
					if v.Err != nil {
						leakErr = v.Err
					}
				}
				val := fullCheckDnsLeakDto(items)
				dnsLeak = &val
				fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
					Task: FullCheckTaskDnsLeak, State: fullCheckErrorState(leakErr), Msg: "dns:leak ready",
				})
			})

			wg.Go(func() {
				items := []DnsVerdict{}
				gch := DnsPlainGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := fullCheckDnsReportDto(items)
				dnsPlain = &val
				fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
					Task: FullCheckTaskDnsPlain, State: fullCheckDnsState(items), Msg: "dns:plain ready",
				})
			})

			wg.Go(func() {
				items := []DnsVerdict{}
				gch := DnsDohGochan(ctx)
				for v := range gch {
					items = append(items, v)
				}
				val := fullCheckDnsReportDto(items)
				dnsDoh = &val
				fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
					Task: FullCheckTaskDnsDoh, State: fullCheckDnsState(items), Msg: "dns:doh ready",
				})
			})
		}

		var webhostMu sync.Mutex
		webhost := map[string]FullCheckWebhostDto{}
		if slices.Contains(cfg.All.Checkers, "webhost") {
			for i, s := range cfg.Checkers.Webhost.Sections {
				wg.Go(func() {
					gch := WebhostGochanRunner(WebhostGochanRunnerOpt{Ctx: ctx, Targets: s.Targets})
					for o := range gch.Out {
						webhostMu.Lock()
						if _, ok := webhost[s.Name]; !ok {
							webhost[s.Name] = FullCheckWebhostDto{Items: []FullCheckWebhostItemDto{}}
						}
						curr := webhost[s.Name]
						curr.Items = append(curr.Items, fullCheckWebhostItemDto(o))
						webhost[s.Name] = curr
						webhostMu.Unlock()
						fullCheckSendProgress(ctx, progressCh, FullCheckProgress{Msg: fmt.Sprintf(`webhost[%s]: "%s" ready`, s.Name, o.Bag.Name)})
					}
					summary := <-gch.Summary
					state := fullCheckErrorState(summary.Err)
					if summary.FarmTimeout {
						state = FullCheckStateTimeout
						webhostMu.Lock()
						curr, ok := webhost[s.Name]
						if !ok {
							curr.Items = []FullCheckWebhostItemDto{}
						}
						curr.FarmTimeout = true
						webhost[s.Name] = curr
						webhostMu.Unlock()
					}
					fullCheckSendProgress(ctx, progressCh, FullCheckProgress{
						Task: FullCheckTaskWebhost, Index: i, State: state, Msg: fmt.Sprintf("webhost[%s] ready", s.Name),
					})
				})
			}
		}

		wg.Wait()
		if ctx.Err() != nil {
			return
		}
		r.Whoami = whoami
		r.CidrWhitelist = cidrwhitelist
		r.Webhost = webhost
		if dnsLeak != nil || dnsPlain != nil || dnsDoh != nil {
			r.Dns = &FullCheckDnsDto{
				Leak:  dnsLeak,
				Plain: dnsPlain,
				Doh:   dnsDoh,
			}
		}

		savePath, err := fullCheckSavePath()
		if err != nil {
			log.Println("fullcheck/savepath:", err)
			fullCheckSendProgress(ctx, progressCh, FullCheckProgress{State: FullCheckStateError, Msg: "error with the output path; enable debug and check the logs"})
			return
		}
		if err := fullCheckSave(savePath, r); err != nil {
			log.Println("fullcheck/save:", err)
			fullCheckSendProgress(ctx, progressCh, FullCheckProgress{State: FullCheckStateError, Msg: "error when saving to a file; enable debug and check the logs"})
			return
		}
		fullCheckSendProgress(ctx, progressCh, FullCheckProgress{State: FullCheckStateOk, Msg: "done; saved to " + savePath})
	}()

	return progressCh

}

func fullCheckDnsState(verdicts []DnsVerdict) FullCheckState {
	for _, v := range verdicts {
		if errors.Is(v.Verdict, ErrDnsProviderTimeout) {
			return FullCheckStateTimeout
		}
	}
	return FullCheckStateOk
}

func fullCheckErrorState(err error) FullCheckState {
	if err == nil {
		return FullCheckStateOk
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FullCheckStateTimeout
	}
	if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		return FullCheckStateTimeout
	}
	return FullCheckStateError
}

func fullCheckSendProgress(ctx context.Context, ch chan<- FullCheckProgress, p FullCheckProgress) {
	if p.Task != FullCheckTaskNone || p.State != FullCheckStateRunning {
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
