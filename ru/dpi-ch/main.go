package main

import (
	"dpich/config"
	"dpich/tui"
	"dpich/webui"
	"flag"
	"log"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	ui := flag.String("ui", "t", "ui mode: t | web")
	cfgPath := flag.String("cfg", "config.yaml", ".yaml config path")
	flag.Parse()

	if err := config.Load(*cfgPath); err != nil {
		log.Fatalf("config load err: %v", err)
	}

	if config.Get().Debug {
		f, err := tea.LogToFile("debug.log", "debug")
		if err != nil {
			log.Fatalf("config load err: %v", err)
		}
		defer f.Close()
	}

	//lab.FarmTest()
	//lab.WebhostCheckerTest()
	//return

	switch *ui {
	case "t":
		tui.Tui()
	case "web":
		webui.Webui()
	default:
		log.Fatalf("unknown --ui value: %s", *ui)
	}
}
