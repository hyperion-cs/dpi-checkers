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

	var cfgPath string
	flag.Func("cfg", "path to a YAML config `file` (default: platform config.yaml)", func(path string) error {
		if path == "" {
			return errors.New("config path must not be empty")
		}
		cfgPath = path
		return nil
	})
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

	if !*ver || cfgPath != "" {
		if err := config.Load(cfgPath); err != nil {
			log.Fatalf("config load err: %v", err)
		}
	}
	if *ver {
		fmt.Println(version.Value)
		return
	}

	if config.Get().Debug {
		debugLog := config.Get().DataPath("debug.log")
		if err := os.MkdirAll(filepath.Dir(debugLog), 0o755); err != nil {
			log.Fatalf("debug log err: %v", err)
		}
		f, err := tea.LogToFile(debugLog, "debug")
		if err != nil {
			log.Fatalf("debug log err: %v", err)
		}
		defer f.Close()
	} else {
		log.SetOutput(io.Discard)
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
