package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/webui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	if err := chdirToBin(); err != nil {
		panic(err)
	}

	ui := flag.String("ui", "t", "ui mode: t | web")
	ver := flag.Bool("version", false, "print version")
	forceInetlookupUpd := flag.Bool("force-inetlookup-update", false, "force run the inetlookup update mechanism")

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

	if *forceInetlookupUpd {
		config.ForceInetlookupUpdate()
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

func chdirToBin() error {
	// Don't change workdir in dev environment
	if version.Value == version.Init {
		return nil
	}
	path, err := os.Executable()
	if err != nil {
		return err
	}
	realPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	return os.Chdir(filepath.Dir(realPath))
}
