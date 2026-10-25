package app_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/app"
	"github.com/m10rten/go-canary-cli/internal/buildinfo"
	"github.com/m10rten/go-canary-cli/internal/channel"
	"github.com/m10rten/go-canary-cli/internal/release"
	"github.com/m10rten/go-canary-cli/test/testutil"
)

func TestUpgrade(t *testing.T) {
	brew, _ := channel.UpgradeCommand(channel.Homebrew)
	npm, _ := channel.UpgradeCommand(channel.NPM)
	choco, _ := channel.UpgradeCommand(channel.Chocolatey)
	dev := buildinfo.Info{Version: "dev", Commit: "none", Date: "unknown", OS: "linux", Arch: "amd64"}
	windows := testInfo
	windows.OS = "windows"

	testcases := []struct {
		name       string
		build      buildinfo.Info
		exe        string
		args       []string
		wantCode   int
		wantStdout string
		stderrHas  string
	}{
		{name: "homebrew prints command", build: testInfo, exe: "/opt/homebrew/Cellar/canary/0.1.3/bin/canary", args: []string{"upgrade"},
			wantStdout: "canary is managed by homebrew. To upgrade, run:\n  " + brew + "\n"},
		{name: "npm prints command", build: testInfo, exe: "/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary", args: []string{"upgrade"},
			wantStdout: "canary is managed by npm. To upgrade, run:\n  " + npm + "\n"},
		{name: "chocolatey prints command", build: windows, exe: `C:\ProgramData\chocolatey\lib\m10rten-canary\tools\canary.exe`, args: []string{"upgrade"},
			wantStdout: "canary is managed by chocolatey. To upgrade, run:\n  " + choco + "\n"},
		{name: "--json does not change output", build: testInfo, exe: "/opt/homebrew/Cellar/canary/0.1.3/bin/canary", args: []string{"upgrade", "--json"},
			wantStdout: "canary is managed by homebrew. To upgrade, run:\n  " + brew + "\n"},
		{name: "homebrew with --version", build: testInfo, exe: "/opt/homebrew/Cellar/canary/0.1.3/bin/canary", args: []string{"upgrade", "--version", "v0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is managed by homebrew (to upgrade, run: " + brew + ")"},
		{name: "npm with --version", build: testInfo, exe: "/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary", args: []string{"upgrade", "--version=0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is managed by npm (to upgrade, run: " + npm + ")"},
		{name: "chocolatey with --version", build: windows, exe: `C:\ProgramData\chocolatey\lib\m10rten-canary\tools\canary.exe`, args: []string{"upgrade", "--version", "v0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is managed by chocolatey (to upgrade, run: " + choco + ")"},
		{name: "unknown dev build", build: dev, exe: "/tmp/canary", args: []string{"upgrade"},
			wantCode: 2, stderrHas: "error: cannot upgrade: install channel is unknown; reinstall canary with install.sh"},
		{name: "unknown with --version", build: dev, exe: "/tmp/canary", args: []string{"upgrade", "--version", "v0.2.0"},
			wantCode: 2, stderrHas: "error: --version only works for install.sh installs, but this install is of unknown origin; reinstall canary with install.sh"},
		{name: "install.sh invalid --version", build: testInfo, exe: "/usr/local/bin/canary", args: []string{"upgrade", "--version", "banana"},
			wantCode: 2, stderrHas: `invalid --version "banana"`},
		{name: "upgrade rejects extra args", build: testInfo, exe: "/usr/local/bin/canary", args: []string{"upgrade", "extra"},
			wantCode: 2, stderrHas: `unexpected argument "extra"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &app.App{
				Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				Executable:   func() (string, error) { return tc.exe, nil },
				EvalSymlinks: func(p string) (string, error) { return p, nil },
			}

			code := a.Run(tc.args, testutil.EnvOf(nil))

			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.stderrHas) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.stderrHas)
			}
		})
	}
}

func TestUpgradeNPMHintPerTrack(t *testing.T) {
	testcases := []struct {
		version string
		want    string
	}{
		{"v0.1.3", "@latest"},
		{"v0.0.0-ci", "@latest"},
		{"dev", "@latest"},
		{"v0.2.0-nightly.20261025", "@nightly"},
		{"v0.2.0-alpha.1", "@alpha"},
		{"v0.2.0-beta.1", "@beta"},
		{"v0.2.0-rc.1", "@rc"},
	}

	for _, tc := range testcases {
		t.Run(tc.version, func(t *testing.T) {
			requests := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()
			build := testInfo
			build.Version = tc.version

			var stdout, stderr bytes.Buffer
			a := &app.App{
				Stdout: &stdout, Stderr: &stderr, Build: build,
				Executable:   func() (string, error) { return "/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary", nil },
				EvalSymlinks: func(p string) (string, error) { return p, nil },
				GitHub:       release.Client{BaseURL: srv.URL, HTTP: srv.Client()},
			}

			code := a.Run([]string{"upgrade"}, testutil.EnvOf(nil))

			want := "canary is managed by npm. To upgrade, run:\n  npm install -g @m10rten/go-canary-cli" + tc.want + "\n"
			if code != 0 || stdout.String() != want {
				t.Errorf("exit code = %d, stdout = %q, want 0 and %q (stderr: %q)", code, stdout.String(), want, stderr.String())
			}
			if requests != 0 {
				t.Errorf("made %d network requests, want none for a managed channel", requests)
			}
		})
	}
}
