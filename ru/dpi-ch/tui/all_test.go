package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func TestAllViewLayout(t *testing.T) {
	spin := spinner.New()
	spin.Spinner = spinner.Line
	model := allModel{
		fetching: true, spinner: spin,
		tasks: map[fullcheck.Task][]allTask{
			fullcheck.TaskWhoami:        {{name: "Whoami", state: fullcheck.StateOk}},
			fullcheck.TaskCidrWhitelist: {{name: "CIDR whitelist", state: fullcheck.StateOk}},
			fullcheck.TaskDnsLeak:       {{name: "Leak", state: fullcheck.StateOk}},
			fullcheck.TaskDnsPlain:      {{name: "Plain"}},
			fullcheck.TaskDnsDoh:        {{name: "DoH", state: fullcheck.StateOk}},
			fullcheck.TaskWebhost: {
				{name: "Popular Web Services", state: fullcheck.StateOk},
				{name: "Foreign Infrastructure Providers"},
				{name: "Russian Infrastructure Providers", state: fullcheck.StateTimeout},
				{name: "Mobile Push Notification Providers"},
			},
		},
		progress: fullcheck.Progress{Msg: `webhost[Foreign Infrastructure Providers]: "Hetzner:de" ready`},
	}
	frame := ansi.Strip(spin.View())
	want := fmt.Sprintf(`Whoami: ✅ done
CIDR whitelist: ✅ done
DNS: { Leak: ✅ done • Plain: %s • DoH: ✅ done }
Webhost: Popular Web Services                ✅ done
         Foreign Infrastructure Providers    %s
         Russian Infrastructure Providers    ⏰ timeout
         Mobile Push Notification Providers  %s

%s webhost[Foreign Infrastructure Providers]: "Hetzner:de" ready`, frame, frame, frame, frame)
	if got := ansi.Strip(allView(model)); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	model.tasks[fullcheck.TaskWebhost] = []allTask{{name: "界", state: fullcheck.StateOk}, {name: "abc", state: fullcheck.StateOk}}
	lines := strings.Split(ansi.Strip(allView(model)), "\n")
	var columns []int
	for _, line := range lines {
		if strings.Contains(line, "界") || strings.Contains(line, "abc") {
			columns = append(columns, lipgloss.Width(line[:strings.Index(line, "✅")]))
		}
	}
	if len(columns) != 2 || columns[0] != columns[1] {
		t.Fatalf("misaligned Unicode names: %v", columns)
	}
	model.progress.Msg = strings.Repeat("long detail ", 30)
	if !strings.Contains(ansi.Strip(allView(model)), model.progress.Msg) {
		t.Fatal("detailed progress was truncated")
	}
	router := NewRouter()
	router.Tab = allTab
	root := rootModel{router: router, allModel: model}
	footer := "\n\nm, backspace: menu • wasd: scroll • q, esc: quit\n"
	renderedLines := strings.Split(ansi.Strip(root.render()), "\n")
	for i := range renderedLines {
		renderedLines[i] = strings.TrimSpace(renderedLines[i])
	}
	if !strings.Contains(strings.Join(renderedLines, "\n"), footer) {
		t.Fatal("missing blank line or existing footer controls")
	}
}

func TestAllCompletionAndCancellation(t *testing.T) {
	cfg := config.Get()
	saved := *cfg
	t.Cleanup(func() { *cfg = saved })
	cfg.All.Checkers = []string{"whoami", "dns", "webhost"}
	cfg.All.AutoExit = false
	cfg.Checkers.Webhost.Sections = []config.WebhostSection{{Name: "same"}, {Name: "same"}}
	model, _ := allUpdate(allModel{}, allInitMsg{})
	defer model.cancel()
	if len(model.tasks) != 5 || len(model.tasks[fullcheck.TaskWebhost]) != 2 {
		t.Fatal("initial task list does not match configuration")
	}
	model, _ = allUpdate(model, msgFor(model.session, allProgressMsg{
		Task: fullcheck.TaskWebhost, Index: 1, State: fullcheck.StateTimeout,
	}))
	webhost := model.tasks[fullcheck.TaskWebhost]
	if webhost[0].state != fullcheck.StateRunning || webhost[1].state != fullcheck.StateTimeout {
		t.Fatal("same-named sections were mixed up")
	}
	model.tasks[fullcheck.TaskWhoami] = append(model.tasks[fullcheck.TaskWhoami], allTask{name: "second"})
	model, _ = allUpdate(model, msgFor(model.session, allProgressMsg{
		Task: fullcheck.TaskWhoami, Index: 1, State: fullcheck.StateError,
	}))
	whoami := model.tasks[fullcheck.TaskWhoami]
	if whoami[0].state != fullcheck.StateRunning || whoami[1].state != fullcheck.StateError {
		t.Fatal("task indexing depends on checker kind")
	}
	model, _ = allUpdate(model, msgFor(model.session, allProducerDoneMsg{}))
	if model.fetching || model.progress.State != fullcheck.StateError {
		t.Fatal("unexpected channel close left progress running")
	}
	for _, tasks := range model.tasks {
		for _, task := range tasks {
			if task.state == fullcheck.StateRunning {
				t.Fatal("task kept spinning after channel close")
			}
		}
	}
	router := NewRouter()
	router.Tab = allTab
	root := rootModel{router: router, allModel: model, viewport: viewport.New(viewport.WithWidth(80), viewport.WithHeight(24))}
	root.Update(exitMsg{})
	if model.ctx.Err() != context.Canceled {
		t.Fatal("leaving the application did not cancel ALL")
	}
}
