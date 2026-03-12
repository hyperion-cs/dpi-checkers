package main

import (
	"dpich/config"
	"dpich/tui"
	"dpich/webui"
	"flag"
	"io"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ui := flag.String("ui", "t", "ui mode: t | web")
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

	switch *ui {
	case "t":
		tui.Tui()
	case "web":
		webui.Webui()
	default:
		log.Fatalf("unknown --ui value: %s", *ui)
	}
}
