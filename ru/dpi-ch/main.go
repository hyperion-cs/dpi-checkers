package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
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
	forceUpd := flag.Bool("force-update", false, "force run the dpi-ch update mechanism")
	all := flag.Bool("all", false, "run all checks immediately (result to file)")

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
	if *forceUpd {
		config.ForceUpdate()
	}
	if *forceInetlookupUpd {
		config.ForceInetlookupUpdate()
	}
	if *all {
		config.RunAllChecksImmediately()
	}

	switch *ui {
	case "t":
		tui.Tui()
	case "web":
		webui.Webui()
	default:
		log.Fatalf("unknown --ui value: %s", *ui)
	}

	// cleaning up some resources
	if c, ok := inetutil.KeyLogWriter().(io.Closer); ok {
		c.Close()
	}
}

func chdirToBin() error {
	// don't change workdir in dev environment
	if version.Value == version.Init {
		return nil
	}

	binFolder, err := config.BinFolder()
	if err != nil {
		return err
	}

	return os.Chdir(binFolder)
}
