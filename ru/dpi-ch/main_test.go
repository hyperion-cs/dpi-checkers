package main

import (
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestEmptyImportAll(t *testing.T) {
	if args := flag.Args(); len(args) > 0 {
		os.Args = append(os.Args[:1], args...)
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
		main()
		return
	}

	// --version keeps this test offline even if the empty import flag is ignored.
	cmd := exec.Command(os.Args[0], "-test.run=^TestEmptyImportAll$", "--", "--import-all=", "--all", "--version")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "report path must not be empty") {
		t.Fatalf("empty --import-all: %v\n%s", err, out)
	}
}
