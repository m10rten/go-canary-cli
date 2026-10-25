package app

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/m10rten/go-canary-cli/internal/buildinfo"
	"github.com/m10rten/go-canary-cli/internal/cli"
	"github.com/m10rten/go-canary-cli/internal/release"
	"github.com/m10rten/go-canary-cli/internal/term"
)

// App holds what command handlers need, so they can be tested with injected
// writers and a fake terminal detector.
type App struct {
	Stdout, Stderr io.Writer
	Build          buildinfo.Info
	Detect         func(jsonOutput, noColor bool) term.Mode
	Executable     func() (string, error)
	EvalSymlinks   func(string) (string, error)
	GitHub         release.Client
	lookupEnv      func(string) (string, bool)
}

// Run executes args and returns the exit code: 0 on success, 1 when check finds
// an update, 2 on any error.
func (a *App) Run(args []string, lookupEnv func(string) (string, bool)) int {
	a.lookupEnv = lookupEnv
	return newRoot(a).Execute(args, lookupEnv, a.Stdout, a.Stderr)
}

// newRoot declares the CLI. To add a command, append a *cli.Command to
// Commands whose Run is a method on App.
func newRoot(a *App) *cli.Root {
	return &cli.Root{
		Command: cli.Command{
			Name:    "canary",
			Summary: "Canary app for testing Go CLI releases, upgrades, and multi-channel distribution.",
			Flags:   []cli.Flag{{Name: "version", Kind: cli.KindBool, Help: "Print version information"}},
			Run:     a.root,
		},
		Globals: []cli.Flag{
			{Name: "json", Kind: cli.KindBool, Help: "Output machine-readable JSON"},
			{Name: "no-color", Kind: cli.KindBool, Env: "NO_COLOR", Help: "Disable colored output"},
		},
		Commands: []*cli.Command{
			{Name: "version", Summary: "Print version, commit, build time and OS/arch", Run: a.version},
			{Name: "info", Summary: "Print executable path, install channel and version", Run: a.info},
			{Name: "check", Summary: "Check for a newer release (exit 1 if one is available)", Run: a.check},
			{
				Name:    "upgrade",
				Summary: "Upgrade to the latest release using the method matching how canary was installed",
				Flags:   []cli.Flag{{Name: "version", Kind: cli.KindString, Help: "Upgrade or downgrade to this release tag (install.sh installs only)"}},
				Run:     a.upgrade,
			},
		},
	}
}

// reporter returns the progress reporter for a command, honoring --json,
// --no-color and NO_COLOR. It writes to stderr.
func (a *App) reporter(ctx *cli.Context) term.Reporter {
	return term.NewReporter(a.Stderr, a.Detect(ctx.Bool("json"), ctx.Bool("no-color")))
}

func (a *App) root(ctx *cli.Context) error {
	if ctx.Bool("version") {
		return a.version(ctx)
	}
	_, err := fmt.Fprintf(a.Stdout, "Hello from canary %s\nRun 'canary help' for usage.\n", a.Build.Version)
	return err
}

func (a *App) version(ctx *cli.Context) error {
	if ctx.Bool("json") {
		return json.NewEncoder(a.Stdout).Encode(a.Build)
	}
	_, err := fmt.Fprintln(a.Stdout, a.Build)
	return err
}
