package checkers

import (
	"context"
	"errors"
	"log"
	"net"

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
