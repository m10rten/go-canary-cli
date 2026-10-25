package app_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/app"
	"github.com/m10rten/go-canary-cli/internal/buildinfo"
	"github.com/m10rten/go-canary-cli/test/testutil"
)

func TestInfo(t *testing.T) {
	release := testInfo
	dev := buildinfo.Info{Version: "dev", Commit: "none", Date: "unknown", OS: "linux", Arch: "amd64"}
	windows := buildinfo.Info{Version: "v0.1.3", Commit: "4f2a9c1", Date: testInfo.Date, OS: "windows", Arch: "amd64"}

	links := map[string]string{
		"/usr/local/bin/canary":    "/opt/homebrew/Cellar/canary/0.1.3/bin/canary",
		"/usr/local/bin/go-canary": "/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary",
	}

	testcases := []struct {
		name       string
		build      buildinfo.Info
		exe        string
		exeErr     error
		linkErr    error
		env        map[string]string
		args       []string
		wantCode   int
		wantStdout string
		stderrHas  string
	}{
		{name: "install.sh text", build: release, exe: "/usr/local/bin/canary-plain", args: []string{"info"},
			wantStdout: "path:    /usr/local/bin/canary-plain\nchannel: install.sh\nversion: v0.1.3\n"},
		{name: "install.sh json", build: release, exe: "/home/me/.local/bin/canary", args: []string{"info", "--json"},
			wantStdout: `{"path":"/home/me/.local/bin/canary","channel":"install.sh","version":"v0.1.3"}` + "\n"},
		{name: "--json before command", build: release, exe: "/home/me/.local/bin/canary", args: []string{"--json", "info"},
			wantStdout: `{"path":"/home/me/.local/bin/canary","channel":"install.sh","version":"v0.1.3"}` + "\n"},
		{name: "symlink resolves to homebrew", build: release, exe: "/usr/local/bin/canary", args: []string{"info", "--json"},
			wantStdout: `{"path":"/opt/homebrew/Cellar/canary/0.1.3/bin/canary","channel":"homebrew","version":"v0.1.3"}` + "\n"},
		{name: "symlink resolves to npm", build: release, exe: "/usr/local/bin/go-canary", args: []string{"info", "--json"},
			wantStdout: `{"path":"/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary","channel":"npm","version":"v0.1.3"}` + "\n"},
		{name: "windows chocolatey default path", build: windows, exe: `C:\ProgramData\chocolatey\lib\m10rten-canary\tools\canary.exe`, args: []string{"info", "--json"},
			wantStdout: `{"path":"C:\\ProgramData\\chocolatey\\lib\\m10rten-canary\\tools\\canary.exe","channel":"chocolatey","version":"v0.1.3"}` + "\n"},
		{name: "windows chocolatey via env", build: windows, exe: `D:\choco\bin\canary.exe`, env: map[string]string{"ChocolateyInstall": `D:\choco`}, args: []string{"info"},
			wantStdout: "path:    D:\\choco\\bin\\canary.exe\nchannel: chocolatey\nversion: v0.1.3\n"},
		{name: "windows npm", build: windows, exe: `C:\Users\me\AppData\Roaming\npm\node_modules\@m10rten\go-canary-cli\canary.exe`, args: []string{"info", "--json"},
			wantStdout: `{"path":"C:\\Users\\me\\AppData\\Roaming\\npm\\node_modules\\@m10rten\\go-canary-cli\\canary.exe","channel":"npm","version":"v0.1.3"}` + "\n"},
		{name: "windows unmanaged is unknown", build: windows, exe: `C:\Users\me\bin\canary.exe`, args: []string{"info"},
			wantStdout: "path:    C:\\Users\\me\\bin\\canary.exe\nchannel: unknown\nversion: v0.1.3\n"},
		{name: "dev build is unknown", build: dev, exe: "/tmp/canary", args: []string{"info", "--json"},
			wantStdout: `{"path":"/tmp/canary","channel":"unknown","version":"dev"}` + "\n"},
		{name: "executable error", build: release, exeErr: errors.New("boom"), args: []string{"info"}, wantCode: 2, stderrHas: "locate executable: boom"},
		{name: "symlink error", build: release, exe: "/usr/local/bin/canary-plain", linkErr: errors.New("loop"), args: []string{"info"}, wantCode: 2, stderrHas: "resolve executable path: loop"},
		{name: "info rejects extra args", build: release, exe: "/tmp/canary", args: []string{"info", "extra"}, wantCode: 2, stderrHas: `unexpected argument "extra"`},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			a := &app.App{
				Stdout: &stdout, Stderr: &stderr, Build: tc.build,
				Executable: func() (string, error) { return tc.exe, tc.exeErr },
				EvalSymlinks: func(p string) (string, error) {
					if tc.linkErr != nil {
						return "", tc.linkErr
					}
					if target, ok := links[p]; ok {
						return target, nil
					}
					return p, nil
				},
			}

			code := a.Run(tc.args, testutil.EnvOf(tc.env))

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
