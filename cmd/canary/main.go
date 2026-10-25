package main

import (
	"os"
	"path/filepath"

	"github.com/m10rten/go-canary-cli/internal/app"
	"github.com/m10rten/go-canary-cli/internal/buildinfo"
	"github.com/m10rten/go-canary-cli/internal/term"
)

func main() {
	a := &app.App{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Build:  buildinfo.Current(),
		Detect: func(jsonOutput, noColor bool) term.Mode {
			return term.Detect(os.Stdout, os.Stderr, jsonOutput, noColor)
		},
		Executable:   os.Executable,
		EvalSymlinks: filepath.EvalSymlinks,
	}
	os.Exit(a.Run(os.Args[1:], os.LookupEnv))
}
