package screens

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/updater"
)

const UpdaterTitle = "Updater"

type Updater struct {
	ctx             context.Context
	err             error
	restartRequired bool
	fetching        bool
	spinner         spinner.Model
	progress        string
}

type updaterErrMsg struct{ err error }
type updaterStartInetlookupMsg struct{}
type updaterSelfDoneMsg struct{ version string }

type UpdaterDoneMsg struct{}

func OpenUpdater(ctx context.Context) (Screen, tea.Cmd) {
	cfg := config.Get()
	selfTtu, _ := updater.TimeToUpdate(cfg.Updater.SelfTsFile)
	inetlookupTtu, _ := updater.TimeToUpdate(cfg.Updater.InetlookupTsFile)
	if !cfg.Updater.ForceUpdate && !cfg.Updater.ForceInetlookupUpdate && !(cfg.Updater.Enabled && (selfTtu || inetlookupTtu)) {
		return nil, nil
	}

	model := &Updater{fetching: true, spinner: primitives.NewSpinner(), ctx: ctx}
	if cfg.Updater.ForceInetlookupUpdate || (cfg.All.Flag && cfg.All.AutoExit && inetlookupTtu) {
		return model, model.startInetlookup()
	}
	if cfg.All.Flag && cfg.All.AutoExit {
		return model, func() tea.Msg { return UpdaterDoneMsg{} }
	}

	model.progress = "checking for updates to itself"
	return model, tea.Batch(
		updaterSelfCmd(ctx, cfg.Updater.ForceUpdate, inetlookupTtu),
		model.spinner.Tick,
	)
}

func (model *Updater) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case updaterStartInetlookupMsg:
		return model.startInetlookup()
	case updaterSelfDoneMsg:
		model.fetching = false
		model.restartRequired = true
		model.progress = msg.version
	case updaterErrMsg:
		model.fetching = false
		model.err = msg.err
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func (model *Updater) startInetlookup() tea.Cmd {
	model.progress = "checking for geoip updates"
	model.fetching = true
	return tea.Batch(updaterInetlookupCmd(model.ctx), model.spinner.Tick)
}

func updaterSelfCmd(ctx context.Context, force, inetlookupTtu bool) tea.Cmd {
	return func() tea.Msg {
		upd, err := updater.SelfCheckUpdates(ctx)
		if err == updater.ErrUnsupportedOsOrArch {
			// TODO: the user should be warned about this.
			return updaterStartInetlookupMsg{}
		}
		if err != nil {
			return updaterErrMsg{err: err}
		}

		if !upd.Required {
			if !force && inetlookupTtu {
				return updaterStartInetlookupMsg{}
			}
			return UpdaterDoneMsg{}
		}

		if err := updater.SelfUpdate(ctx, upd.AssetUrl, upd.AssetFilename, upd.AssetVersion); err != nil {
			return updaterErrMsg{err: err}
		}
		return updaterSelfDoneMsg{version: upd.AssetVersion}
	}
}

func updaterInetlookupCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		if err := updater.GeoliteUpdate(ctx); err != nil {
			return updaterErrMsg{err: err}
		}
		return UpdaterDoneMsg{}
	}
}

func isTimeoutErr(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	ne, ok := errors.AsType[net.Error](err)
	return ok && ne.Timeout()
}

func (model *Updater) View() string {
	if model.err != nil {
		if isTimeoutErr(model.err) {
			return "⚠️ error: network timeout in the update mechanism; you can skip this step or restart the utility"
		}
		return fmt.Sprintf("⚠️ error: %v", model.err)
	}
	if model.fetching {
		return fmt.Sprintf("%s %s\n", model.spinner.View(), model.progress)
	}
	if model.restartRequired {
		return fmt.Sprintf("✅ dpi-ch utility has been updated (=> %s). Please restart.", model.progress)
	}
	return "noop"
}
