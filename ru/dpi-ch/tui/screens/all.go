package screens

import (
	"cmp"
	"context"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui/primitives"
)

type All struct {
	fetching bool
	spinner  spinner.Model
	progress fullcheck.Progress
	out      <-chan fullcheck.Progress
	tasks    map[fullcheck.Task][]allTask
}

type allTask struct {
	name  string
	state fullcheck.State
}

type AllExitMsg struct{}

type allProducerStartedMsg struct {
	out <-chan fullcheck.Progress
}

type allProgressMsg fullcheck.Progress
type allProducerDoneMsg struct{}

func OpenAll(ctx context.Context) (Screen, tea.Cmd) {
	model := &All{
		fetching: true,
		spinner:  primitives.NewSpinner(),
		tasks:    make(map[fullcheck.Task][]allTask),
	}
	cfg := config.Get()
	for _, name := range cfg.All.Checkers {
		switch name {
		case "whoami":
			model.tasks[fullcheck.TaskWhoami] = []allTask{{name: "Whoami"}}
		case "cidrwhitelist":
			model.tasks[fullcheck.TaskCidrWhitelist] = []allTask{{name: "CIDR whitelist"}}
		case "dns":
			model.tasks[fullcheck.TaskDnsLeak] = []allTask{{name: "Leak"}}
			model.tasks[fullcheck.TaskDnsPlain] = []allTask{{name: "Plain"}}
			model.tasks[fullcheck.TaskDnsDoh] = []allTask{{name: "DoH"}}
		case "webhost":
			tasks := make([]allTask, len(cfg.Checkers.Webhost.Sections))
			for i, section := range cfg.Checkers.Webhost.Sections {
				tasks[i].name = section.Name
			}
			model.tasks[fullcheck.TaskWebhost] = tasks
		}
	}
	return model, tea.Batch(model.spinner.Tick, func() tea.Msg {
		return allProducerStartedMsg{out: fullcheck.Gochan(ctx)}
	})
}

func (model *All) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case allProducerStartedMsg:
		model.out = msg.out
		return allConsumerCmd(model.out)
	case allProgressMsg:
		progress := fullcheck.Progress(msg)
		if progress.Msg != "" {
			model.progress = progress
		}
		tasks := model.tasks[progress.Task]
		if progress.Index >= 0 && progress.Index < len(tasks) {
			tasks[progress.Index].state = progress.State
		}
		return allConsumerCmd(model.out)
	case allProducerDoneMsg:
		model.fetching = false
		for _, tasks := range model.tasks {
			for i := range tasks {
				if tasks[i].state == fullcheck.StateRunning {
					tasks[i].state = fullcheck.StateError
				}
			}
		}
		if model.progress.Task != fullcheck.TaskNone || model.progress.State == fullcheck.StateRunning {
			model.progress = fullcheck.Progress{State: fullcheck.StateError, Msg: "stopped before completion"}
		}
		if config.Get().All.AutoExit {
			return func() tea.Msg { return AllExitMsg{} }
		}
	case spinner.TickMsg:
		if model.fetching {
			var cmd tea.Cmd
			model.spinner, cmd = model.spinner.Update(msg)
			return cmd
		}
	}
	return nil
}

func allConsumerCmd(out <-chan fullcheck.Progress) tea.Cmd {
	return func() tea.Msg {
		if out != nil {
			if progress, ok := <-out; ok {
				return allProgressMsg(progress)
			}
		}
		return allProducerDoneMsg{}
	}
}

func (model *All) View() string {
	var lines []string
	frame := model.spinner.View()
	for _, kind := range []fullcheck.Task{fullcheck.TaskWhoami, fullcheck.TaskCidrWhitelist} {
		for _, task := range model.tasks[kind] {
			lines = append(lines, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	var dns []string
	for _, kind := range []fullcheck.Task{fullcheck.TaskDnsLeak, fullcheck.TaskDnsPlain, fullcheck.TaskDnsDoh} {
		for _, task := range model.tasks[kind] {
			dns = append(dns, task.name+": "+allStatusView(task.state, frame, ""))
		}
	}
	if len(dns) > 0 {
		lines = append(lines, "DNS: { "+strings.Join(dns, primitives.DotChar)+" }")
	}
	webhost := model.tasks[fullcheck.TaskWebhost]
	width := 0
	for _, task := range webhost {
		width = max(width, lipgloss.Width(task.name))
	}
	for i, task := range webhost {
		prefix := "         "
		if i == 0 {
			prefix = "Webhost: "
		}
		padding := strings.Repeat(" ", width-lipgloss.Width(task.name)+2)
		lines = append(lines, prefix+task.name+padding+allStatusView(task.state, frame, ""))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	if model.progress.Msg == "" && !model.fetching {
		lines = append(lines, "idle")
	} else {
		state := model.progress.State
		if model.fetching && model.progress.Task != fullcheck.TaskNone {
			state = fullcheck.StateRunning
		}
		lines = append(lines, allStatusView(state, frame, cmp.Or(model.progress.Msg, "fetching")))
	}
	return strings.Join(lines, "\n")
}

func allStatusView(state fullcheck.State, frame, msg string) string {
	switch state {
	case fullcheck.StateOk:
		return primitives.OkStyle.Render("✅ " + cmp.Or(msg, "done"))
	case fullcheck.StateError:
		return primitives.DangerStyle.Render("❌ " + cmp.Or(msg, "error"))
	case fullcheck.StateTimeout:
		return primitives.WarningStyle.Render("⏰ " + cmp.Or(msg, "timeout"))
	default:
		if msg == "" {
			return frame
		}
		return frame + " " + msg
	}
}
