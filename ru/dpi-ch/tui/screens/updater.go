package screens

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
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
	updateGeoLite := cfg.Updater.ForceInetlookupUpdate || (cfg.Updater.Enabled && updater.GeoliteDue())
	var updateSelf bool
	if !cfg.Updater.ForceInetlookupUpdate && !config.Container() && !(cfg.All.Flag && cfg.All.AutoExit) {
		updateSelf = cfg.Updater.ForceUpdate || (cfg.Updater.Enabled && updater.SelfUpdateDue())
	}
	if !updateSelf && !updateGeoLite {
		return nil, nil
	}

	model := &Updater{fetching: true, spinner: primitives.NewSpinner(), ctx: ctx}
	if !updateSelf {
		return model, model.startInetlookup()
	}

	model.progress = "checking for updates to itself"
	return model, tea.Batch(
		updaterSelfCmd(ctx, cfg.Updater.ForceUpdate, updateGeoLite),
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

func updaterSelfCmd(ctx context.Context, force, updateGeoLite bool) tea.Cmd {
	return func() tea.Msg {
		upd, err := updater.SelfCheckUpdates(ctx)
		unsupported := errors.Is(err, updater.ErrUnsupportedOsOrArch)
		if err != nil && !unsupported {
			if force {
				return updaterErrMsg{err: err}
			}
			log.Printf("updater/self: %v", err)
		}
		if err != nil || !upd.Required {
			if updateGeoLite && (unsupported || !force) {
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
			if ctx.Err() != nil {
				return updaterErrMsg{err: err}
			}
			if lookupErr := inetlookup.InitDefault(); lookupErr != nil {
				return updaterErrMsg{err: errors.Join(err, lookupErr)}
			}
			log.Printf("updater/geolite: using local database after update failed: %v", err)
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
			return "⚠️ error: network timeout while checking for updates"
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
