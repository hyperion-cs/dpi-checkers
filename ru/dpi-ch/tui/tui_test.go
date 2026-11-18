package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetlookup"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/screens"
)

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	return cfg
}

func lookupFixture(cfg *config.Config) {
	const dir = "../inetlookup/testdata/geolite2_csv/"
	cfg.InetlookupGeolitecsv.CidrAs = dir + "cidr2as_ipv4.csv"
	cfg.InetlookupGeolitecsv.CidrCountry = dir + "cidr2countryIso_ipv4.csv"
	cfg.InetlookupGeolitecsv.GeonameidCountry = dir + "geonameId2country_en.csv"
}

func update(rm rootModel, msg tea.Msg) (rootModel, tea.Cmd) {
	next, cmd := rm.Update(msg)
	return next.(rootModel), cmd
}

func TestLiveTabInitialization(t *testing.T) {
	cfg := testConfig(t)
	lookupFixture(cfg)
	inetlookup.Default()
	cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "example section"}}
	cfg.Checkers.Webhost.FarmTimeout = 47 * time.Second

	for i, title := range []string{"ALL", "Who am I?", "Am I under the CIDR whitelist?", "example section", "DNS"} {
		t.Run(title, func(t *testing.T) {
			model := newRootModel(TuiOpt{})
			for range i {
				model, _ = update(model, tea.KeyPressMsg{Code: tea.KeyDown})
			}
			model, cmd := update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || model.session == nil || model.title != title {
				t.Fatal("live menu did not open the screen and schedule its commands")
			}
			rendered := ansi.Strip(model.render())
			if !strings.Contains(rendered, "Tab: "+title) || !strings.Contains(rendered, "m, backspace: menu • wasd: scroll • q, esc: quit") {
				t.Fatalf("screen lost its title or footer controls: %s", rendered)
			}
			model.quit()
		})
	}
}

func TestClosingScreenCancelsItsCommands(t *testing.T) {
	cfg := testConfig(t)
	lookupFixture(cfg)
	inetlookup.Default()
	model := newRootModel(TuiOpt{})
	model, start := update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	run := model.session
	batch, ok := start().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatal("screen did not schedule its spinner and check as a batch")
	}
	tick := batch[0]()
	if message, ok := tick.(sessionMsg); !ok || message.session != run {
		t.Fatal("batch command was not scoped to the screen")
	} else if _, ok := message.msg.(spinner.TickMsg); !ok {
		t.Fatal("screen did not schedule a spinner tick")
	}

	model, _ = update(model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if run.ctx.Err() != context.Canceled || model.screen != nil {
		t.Fatal("backspace did not close and cancel the check")
	}
	if msg := batch[1](); msg != nil {
		t.Fatal("a check scheduled before backspace started after cancellation")
	}

	model, _ = update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.session == nil || model.session == run {
		t.Fatal("reopening the screen did not start a new session")
	}
	model, cmd := update(model, sessionMsg{run, screens.AllExitMsg{}})
	if cmd != nil || model.quitting {
		t.Fatal("a delayed exit command quit the reopened ALL screen")
	}
	run = model.session
	model, cmd = update(model, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil || !model.quitting || run.ctx.Err() != context.Canceled {
		t.Fatal("quit did not cancel the active task")
	}
}

func TestQuitBeforeStartup(t *testing.T) {
	cfg := testConfig(t)
	cfg.Updater.ForceUpdate = true
	model := newRootModel(TuiOpt{})
	startup := model.Init()()
	model, quit := update(model, tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !model.quitting || quit == nil {
		t.Fatal("quit did not begin")
	}
	model, cmd := update(model, startup)
	if model.screen != nil || model.session != nil || cmd != nil {
		t.Fatal("queued startup started work after quit")
	}
}

func TestUpdaterContinuesToAll(t *testing.T) {
	cfg := testConfig(t)
	lookupFixture(cfg)
	cfg.Updater.Enabled = true
	cfg.Updater.RootDir = t.TempDir()
	cfg.Updater.InetlookupTsFile = "geoip.ts"
	cfg.Updater.ForceUpdate, cfg.Updater.ForceInetlookupUpdate = false, false
	cfg.All.Flag, cfg.All.AutoExit = true, true

	model := newRootModel(TuiOpt{})
	model, cmd := update(model, model.Init()())
	if _, ok := model.screen.(*screens.Updater); !ok || cmd == nil {
		t.Fatal("startup did not select the updater")
	}
	updaterRun := model.session
	model, cmd = update(model, sessionMsg{updaterRun, screens.UpdaterDoneMsg{}})
	if model.screen != nil || updaterRun.ctx.Err() != context.Canceled || cmd == nil {
		t.Fatal("updater completion did not schedule initialization for ALL")
	}
	ready := cmd()
	if batch, ok := ready.(tea.BatchMsg); ok {
		ready = batch[0]()
	}
	model, cmd = update(model, ready)
	if _, ok := model.screen.(*screens.All); !ok || cmd == nil {
		t.Fatal("lookup initialization did not open ALL")
	}

	run := model.session
	var lines []string
	for _, line := range strings.Split(ansi.Strip(model.render()), "\n") {
		lines = append(lines, strings.TrimSpace(line))
	}
	const footer = "\n\nm, backspace: menu • wasd: scroll • q, esc: quit\n"
	if !strings.Contains(strings.Join(lines, "\n"), footer) {
		t.Fatal("ALL lost the blank line or footer controls")
	}
	model, cmd = update(model, sessionMsg{run, screens.AllExitMsg{}})
	if cmd == nil || !model.quitting || run.ctx.Err() != context.Canceled {
		t.Fatal("ALL exit did not cancel the active task")
	}
}

func TestPathsToggleInLastRow(t *testing.T) {
	cfg := testConfig(t)
	lookupFixture(cfg)
	inetlookup.Default()
	model := newRootModel(TuiOpt{})
	model.configPath = "/config/dpich/config.yaml"
	model.dataPath = "/data/dpich"
	checkFooter := func(path string) {
		t.Helper()
		const label = " · p: config/data · path: "
		rendered := strings.TrimSpace(ansi.Strip(model.render()))
		last := rendered[strings.LastIndexByte(rendered, '\n')+1:]
		if !strings.HasSuffix(last, label+path) || strings.Count(rendered, label) != 1 {
			t.Fatalf("expected %q only in the last row, got %q", path, last)
		}
	}
	checkFooter(model.configPath)
	model, _ = update(model, tea.KeyPressMsg{Code: 'p', Text: "p"})
	checkFooter(model.dataPath)

	model, _ = update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if model.screen == nil {
		t.Fatal("main menu did not open a checker")
	}
	checkFooter(model.dataPath)
	model, _ = update(model, tea.KeyPressMsg{Code: 'з', Text: "з"})
	checkFooter(model.configPath)

	model, _ = update(model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	checkFooter(model.configPath)
}

func TestPathsFooterHighlightsOnlyUnwrittenConfig(t *testing.T) {
	model := newRootModel(TuiOpt{})
	model.configPath = "/config/dpich/config.yaml"
	model.dataPath = "/data/dpich"
	model.configCreateFailed = true
	if footer := model.renderFooter(); !strings.HasSuffix(footer, primitives.DangerStyle.Render(model.configPath)) {
		t.Fatalf("missing config path should be highlighted: %q", footer)
	}
	model.showDataPath = true
	if footer := model.renderFooter(); !strings.HasSuffix(footer, primitives.SubtleStyle.Render(model.dataPath)) {
		t.Fatalf("data path should use its normal style: %q", footer)
	}
}

func TestLookupFailureKeepsAllFromStarting(t *testing.T) {
	model := newRootModel(TuiOpt{})
	model.session = newSession()
	run := model.session
	err := errors.New("invalid GeoLite CSV")
	model, cmd := update(model, sessionMsg{run, lookupReadyMsg{all: true, err: err}})
	if cmd != nil || model.screen != nil || run.ctx.Err() != context.Canceled {
		t.Fatal("lookup failure did not cancel startup before ALL")
	}
	if text := ansi.Strip(model.render()); !strings.Contains(text, err.Error()) {
		t.Fatalf("lookup failure is missing from the TUI: %q", text)
	}
	model, cmd = update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || model.screen != nil {
		t.Fatal("lookup failure allowed a checker to start")
	}
}

func TestReportStaysOffline(t *testing.T) {
	cfg := testConfig(t)
	cfg.Updater.ForceUpdate, cfg.Updater.ForceInetlookupUpdate = true, true
	cfg.All.Flag = true
	model := newRootModel(TuiOpt{
		Report: &fullcheck.ReportDto{
			Whoami: &fullcheck.WhoamiDto{Ip: "192.0.2.1", Status: fullcheck.StatusDto{Reason: "NONE"}},
		},
		ReportPath: "/reports/saved.yml",
	})
	if model.Init() != nil || !strings.Contains(ansi.Strip(model.render()), "Results from: /reports/saved.yml") {
		t.Fatal("report viewer entered live initialization")
	}
	model, cmd := update(model, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || model.screen == nil || !strings.Contains(model.screen.View(), "192.0.2.1") {
		t.Fatal("report viewer did not open the saved screen synchronously")
	}
	for _, msg := range []tea.Msg{startupMsg{}, lookupReadyMsg{all: true}, screens.UpdaterDoneMsg{}, screens.AllExitMsg{}} {
		next, cmd := update(model, msg)
		if cmd != nil || next.screen != model.screen || next.quitting {
			t.Fatal("live message affected an open report")
		}
		model = next
	}
	model, cmd = update(model, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if cmd != nil || model.screen != nil || !strings.Contains(ansi.Strip(model.render()), "Results from: /reports/saved.yml") {
		t.Fatal("returning to the report menu scheduled work")
	}
}
