package tui

import (
	"context"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/updater"

	tea "charm.land/bubbletea/v2"
)

func (rm rootModel) Init() tea.Cmd {
	updaterCfg := config.Get().Updater
	ttu, _ := updater.TimeToUpdate()
	if updaterCfg.ForceInetlookupUpdate || (updaterCfg.Enabled && ttu) {
		return func() tea.Msg {
			return updaterInitMsg{forceInetlookupUpdate: updaterCfg.ForceInetlookupUpdate}
		}
	}

	// if updates are disabled, then user is responsible for maintaining current state
	return func() tea.Msg {
		inetlookup.Default()
		return nil
	}
}

func whoamiFetchCmd() tea.Msg {
	res, err := checkers.Whoami()
	return whoamiResultMsg{res, err}
}

func cidrwhitelistCheckCmd() tea.Msg {
	err := checkers.CidrWhitelist()
	return cidrwhitelistResultMsg{err: err}
}

func webhostProducerStartCmd(ctx context.Context, mode checkers.WebHostMode) tea.Cmd {
	return func() tea.Msg {
		opt := checkers.WebhostGochanRunnerOpt{Ctx: ctx, Mode: mode}
		out := checkers.WebhostGochanRunner(opt)
		return webhostProducerStartedMsg{out}
	}
}

func webhostConsumerCmd(out checkers.WebhostGochanRunnerOut) tea.Cmd {
	return func() tea.Msg {
		for out.Out != nil || out.Progress != nil {
			select {
			case v, ok := <-out.Out:
				if !ok {
					out.Out = nil
					continue
				}
				return webhostItemMsg(v)
			case v, ok := <-out.Progress:
				if !ok {
					out.Progress = nil
					continue
				}
				return webhostProgressMsg(v)
			}
		}

		return webhostProducerDoneMsg{}
	}
}

func dnsProducerStartCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		return dnsProducerStartedMsg{
			out: dnsChannelModel{
				leak:          checkers.DnsLeakGochan(ctx),
				providerPlain: checkers.DnsPlainGochan(ctx),
				providerDoh:   checkers.DnsDohGochan(ctx),
				progress:      make(chan string, 16),
			},
		}
	}
}

func dnsConsumerCmd(out dnsChannelModel) tea.Cmd {
	return func() tea.Msg {
		for out.providerPlain != nil || out.providerDoh != nil || out.leak != nil {
			select {
			case v, ok := <-out.providerPlain:
				if !ok {
					out.providerPlain = nil
					continue
				}
				return dnsProviderPlainMsg(v)
			case v, ok := <-out.providerDoh:
				if !ok {
					out.providerDoh = nil
					continue
				}
				return dnsProviderDohMsg(v)
			case v, ok := <-out.leak:
				if !ok {
					out.leak = nil
					continue
				}
				return dnsLeakMsg(v)
			case v := <-out.progress:
				return dnsProgressMsg(v)
			}
		}
		return dnsProducerDoneMsg{}
	}
}

func updaterSelfCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		upd, err := updater.SelfCheckUpdates(ctx)
		if err == updater.ErrUnsupportedOsOrArch {
			// TODO: the user should be warned about this.
			return updaterSelfNoopMsg{}
		}

		if err != nil {
			return updaterErrMsg{err: err}
		}

		if !upd.Required {
			return updaterSelfNoopMsg{}
		}

		if err = updater.SelfUpdate(ctx, upd.Name, upd.Url); err != nil {
			return updaterErrMsg{err: err}
		}

		return updaterSelfDoneMsg{name: upd.Name}
	}
}

func updaterInetlookupCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if err := updater.GeoliteUpdate(ctx); err != nil {
			return updaterErrMsg{err: err}
		}
		inetlookup.Default()
		return updaterDoneMsg{}
	}
}
