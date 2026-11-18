package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/checkers/fullcheck"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/config"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/inetutil"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/internal/version"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/tui"
	"github.com/hyperion-cs/dpi-checkers/ru/dpi-ch/webui"

	tea "charm.land/bubbletea/v2"
)

func main() {
	ui := flag.String("ui", "t", "ui mode: t | web")
	ver := flag.Bool("version", false, "print version")
	forceInetlookupUpd := flag.Bool("force-inetlookup-update", false, "force run the inetlookup update mechanism")
	forceUpd := flag.Bool("force-update", false, "force run the dpi-ch update mechanism")
	all := flag.Bool("all", false, "run all checks immediately (result to file)")
	var importAll string
	flag.Func("import-all", "view an ALL report from a JSON/YAML `file`", func(path string) error {
		if path == "" {
			return errors.New("report path must not be empty")
		}
		importAll = path
		return nil
	})

	cfgPath := flag.String("cfg", config.CfgDefPath, ".yaml config path")
	flag.Parse()

	var tuiOpt tui.TuiOpt
	if importAll != "" {
		if *all || *forceUpd || *forceInetlookupUpd || *ui != "t" {
			log.Fatal("--import-all requires --ui=t and cannot be combined with --all or forced updates")
		}
		file, err := filepath.Abs(importAll)
		if err != nil {
			log.Fatalf("import path: %v", err)
		}
		report, err := fullcheck.Import(file)
		if err != nil {
			log.Fatalf("import report: %v", err)
		}
		tuiOpt = tui.TuiOpt{Report: report, ReportPath: file}
	}
	if err := chdirToBin(); err != nil {
		panic(err)
	}

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

	// this is ignored if the ALL + auto-exit flags are set
	if *forceUpd {
		config.ForceUpdateFlag()
	}

	if *forceInetlookupUpd {
		config.ForceInetlookupUpdateFlag()
	}

	if *all {
		config.AllFlag()
	}

	switch *ui {
	case "t":
		tui.Tui(tuiOpt)
	case "web":
		webui.Webui()
	default:
		log.Fatalf("unknown --ui value: %s", *ui)
	}

	// cleaning up some resources
	if tuiOpt.Report == nil {
		if c, ok := inetutil.KeyLogWriter().(io.Closer); ok {
			c.Close()
		}
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
