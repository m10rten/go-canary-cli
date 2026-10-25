package app_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/app"
	"github.com/m10rten/go-canary-cli/internal/buildinfo"
	"github.com/m10rten/go-canary-cli/internal/release"
	"github.com/m10rten/go-canary-cli/internal/term"
	"github.com/m10rten/go-canary-cli/test/testutil"
)

var testInfo = buildinfo.Info{
	Version: "v0.1.3",
	Commit:  "4f2a9c1",
	Date:    "2026-10-25T14:02:11Z",
	OS:      "darwin",
	Arch:    "arm64",
}

const (
	versionLine = "canary v0.1.3 (commit 4f2a9c1, built 2026-10-25T14:02:11Z, darwin/arm64)\n"
	versionJSON = `{"version":"v0.1.3","commit":"4f2a9c1","build_time":"2026-10-25T14:02:11Z","os":"darwin","arch":"arm64"}` + "\n"
	greeting    = "Hello from canary v0.1.3\nRun 'canary help' for usage.\n"
)

func TestRun(t *testing.T) {
	testcases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStdout string   // exact match when non-empty
		stdoutHas  []string // substrings
		stderrHas  string
	}{
		{name: "root greeting", wantStdout: greeting},
		{name: "version text", args: []string{"version"}, wantStdout: versionLine},
		{name: "root --version", args: []string{"--version"}, wantStdout: versionLine},
		{name: "version --json", args: []string{"version", "--json"}, wantStdout: versionJSON},
		{name: "--json version", args: []string{"--json", "version"}, wantStdout: versionJSON},
		{name: "--version --json", args: []string{"--version", "--json"}, wantStdout: versionJSON},
		{name: "--no-color after command", args: []string{"version", "--no-color"}, wantStdout: versionLine},
		{name: "--no-color before command", args: []string{"--no-color", "version"}, wantStdout: versionLine},
		{name: "greeting ignores --json", args: []string{"--json"}, wantStdout: greeting},
		{name: "help", args: []string{"help"}, stdoutHas: []string{"canary [command] [flags]", "version", "--json", "--no-color", "env NO_COLOR"}},
		{name: "version --help", args: []string{"version", "--help"}, stdoutHas: []string{"canary version [flags]", "--json"}},
		{name: "unknown flag", args: []string{"--bogus"}, wantCode: 2, stderrHas: "unknown flag --bogus"},
		{name: "unknown command", args: []string{"bogus"}, wantCode: 2, stderrHas: `unknown command "bogus"`},
		{name: "version rejects root flag", args: []string{"version", "--version"}, wantCode: 2, stderrHas: "unknown flag --version"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &app.App{Stdout: &stdout, Stderr: &stderr, Build: testInfo}

			code := a.Run(tc.args, testutil.EnvOf(nil))

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if tc.wantStdout != "" && stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			for _, s := range tc.stdoutHas {
				if !strings.Contains(stdout.String(), s) {
					t.Errorf("stdout missing %q:\n%s", s, stdout.String())
				}
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
			if tc.wantCode != 0 && stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty on error", stdout.String())
			}
		})
	}
}

func TestReporterReceivesGlobalFlags(t *testing.T) {
	testcases := []struct {
		name        string
		args        []string
		env         map[string]string
		wantJSON    bool
		wantNoColor bool
	}{
		{name: "defaults", args: []string{"check"}},
		{name: "--json before command", args: []string{"--json", "check"}, wantJSON: true},
		{name: "--json after command", args: []string{"check", "--json"}, wantJSON: true},
		{name: "--no-color before command", args: []string{"--no-color", "check"}, wantNoColor: true},
		{name: "--no-color after command", args: []string{"check", "--no-color"}, wantNoColor: true},
		{name: "NO_COLOR set", args: []string{"check"}, env: map[string]string{"NO_COLOR": "1"}, wantNoColor: true},
		{name: "NO_COLOR empty", args: []string{"check"}, env: map[string]string{"NO_COLOR": ""}},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"tag_name":"v0.1.3"}`))
			}))
			defer srv.Close()

			var gotJSON, gotNoColor bool
			a := &app.App{
				Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Build: testInfo,
				Detect: func(j, nc bool) term.Mode {
					gotJSON, gotNoColor = j, nc
					return term.Mode{}
				},
				GitHub: release.Client{BaseURL: srv.URL, HTTP: srv.Client()},
			}

			if code := a.Run(tc.args, testutil.EnvOf(tc.env)); code != 0 {
				t.Fatalf("exit code = %d, want 0", code)
			}

			if gotJSON != tc.wantJSON || gotNoColor != tc.wantNoColor {
				t.Errorf("Detect(json=%v, noColor=%v), want (%v, %v)", gotJSON, gotNoColor, tc.wantJSON, tc.wantNoColor)
			}
		})
	}
}
