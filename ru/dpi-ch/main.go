package main

import (
	"dpich/config"
	"dpich/internal/version"
	"dpich/tui"
	"dpich/updater"
	"dpich/webui"
	"flag"
	"fmt"
	"io"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ui := flag.String("ui", "t", "ui mode: t | web")
	ver := flag.Bool("version", false, "print version")
	upd := flag.Bool("update", false, "update executable")
	cfgPath := flag.String("cfg", config.CfgDefPath, ".yaml config path")
	flag.Parse()

	if err := config.Load(*cfgPath); err != nil {
		log.Fatalf("config load err: %v", err)
	}

	log.SetOutput(io.Discard)
	if config.Get().Debug {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			log.Fatalf("debug log err: %v", err)
		}
		defer f.Close()
	}

	if *ver {
		fmt.Println(version.Value)
		return
	}

	if *upd {
		updater.SelfUpdateExecutable(flag.Arg(0), flag.Arg(1))
	}

	switch *ui {
	case "t":
		tui.Tui()
	case "web":
		webui.Webui()
	default:
		log.Fatalf("unknown --ui value: %s", *ui)
	}
}
