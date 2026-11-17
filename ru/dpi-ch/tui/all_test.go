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
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
)

func TestAllViewLayout(t *testing.T) {
	spin := spinner.New()
	spin.Spinner = spinner.Line
	model := allModel{
		fetching: true, spinner: spin,
		tasks: map[checkers.FullCheckTask][]allTask{
			checkers.FullCheckTaskWhoami:        {{name: "Whoami", state: checkers.FullCheckStateOk}},
			checkers.FullCheckTaskCidrWhitelist: {{name: "CIDR whitelist", state: checkers.FullCheckStateOk}},
			checkers.FullCheckTaskDnsLeak:       {{name: "Leak", state: checkers.FullCheckStateOk}},
			checkers.FullCheckTaskDnsPlain:      {{name: "Plain"}},
			checkers.FullCheckTaskDnsDoh:        {{name: "DoH", state: checkers.FullCheckStateOk}},
			checkers.FullCheckTaskWebhost: {
				{name: "Popular Web Services", state: checkers.FullCheckStateOk},
				{name: "Foreign Infrastructure Providers"},
				{name: "Russian Infrastructure Providers", state: checkers.FullCheckStateTimeout},
				{name: "Mobile Push Notification Providers"},
			},
		},
		progress: checkers.FullCheckProgress{Msg: `webhost[Foreign Infrastructure Providers]: "Hetzner:de" ready`},
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
	model.tasks[checkers.FullCheckTaskWebhost] = []allTask{{name: "界", state: checkers.FullCheckStateOk}, {name: "abc", state: checkers.FullCheckStateOk}}
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
	if len(model.tasks) != 5 || len(model.tasks[checkers.FullCheckTaskWebhost]) != 2 {
		t.Fatal("initial task list does not match configuration")
	}
	model, _ = allUpdate(model, msgFor(model.session, allProgressMsg{
		Task: checkers.FullCheckTaskWebhost, Index: 1, State: checkers.FullCheckStateTimeout,
	}))
	webhost := model.tasks[checkers.FullCheckTaskWebhost]
	if webhost[0].state != checkers.FullCheckStateRunning || webhost[1].state != checkers.FullCheckStateTimeout {
		t.Fatal("same-named sections were mixed up")
	}
	model.tasks[checkers.FullCheckTaskWhoami] = append(model.tasks[checkers.FullCheckTaskWhoami], allTask{name: "second"})
	model, _ = allUpdate(model, msgFor(model.session, allProgressMsg{
		Task: checkers.FullCheckTaskWhoami, Index: 1, State: checkers.FullCheckStateError,
	}))
	whoami := model.tasks[checkers.FullCheckTaskWhoami]
	if whoami[0].state != checkers.FullCheckStateRunning || whoami[1].state != checkers.FullCheckStateError {
		t.Fatal("task indexing depends on checker kind")
	}
	model, _ = allUpdate(model, msgFor(model.session, allProducerDoneMsg{}))
	if model.fetching || model.progress.State != checkers.FullCheckStateError {
		t.Fatal("unexpected channel close left progress running")
	}
	for _, tasks := range model.tasks {
		for _, task := range tasks {
			if task.state == checkers.FullCheckStateRunning {
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
