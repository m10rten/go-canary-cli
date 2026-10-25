package channel_test

import (
	"testing"

	"github.com/m10rten/go-canary-cli/internal/channel"
	"github.com/m10rten/go-canary-cli/test/testutil"
)

func TestDetect(t *testing.T) {
	chocoEnv := map[string]string{"ChocolateyInstall": `C:\ProgramData\chocolatey`}

	testcases := []struct {
		name      string
		path      string
		goos      string
		isRelease bool
		env       map[string]string
		want      channel.Channel
	}{
		{name: "homebrew macOS arm64", path: "/opt/homebrew/Cellar/canary/0.1.3/bin/canary", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "homebrew macOS intel", path: "/usr/local/Cellar/canary/0.1.3/bin/canary", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "linuxbrew", path: "/home/linuxbrew/.linuxbrew/Cellar/canary/0.1.3/bin/canary", goos: "linux", isRelease: true, want: channel.Homebrew},
		{name: "homebrew caskroom", path: "/opt/homebrew/Caskroom/canary/0.1.3/canary", goos: "darwin", isRelease: true, want: channel.Homebrew},
		{name: "npm global unix", path: "/usr/local/lib/node_modules/@m10rten/go-canary-cli/bin/canary", goos: "linux", isRelease: true, want: channel.NPM},
		{name: "npm global windows", path: `C:\Users\me\AppData\Roaming\npm\node_modules\@m10rten\go-canary-cli\bin\canary.exe`, goos: "windows", isRelease: true, want: channel.NPM},
		{name: "chocolatey via env var", path: `C:\ProgramData\chocolatey\bin\canary.exe`, goos: "windows", isRelease: true, env: chocoEnv, want: channel.Chocolatey},
		{name: "chocolatey env var in other case and separators", path: "c:/programdata/CHOCOLATEY/bin/canary.exe", goos: "windows", isRelease: true, env: map[string]string{"ChocolateyInstall": `C:\ProgramData\chocolatey\`}, want: channel.Chocolatey},
		{name: "chocolatey custom root via env var", path: `D:\tools\choco\lib\m10rten-canary\tools\canary.exe`, goos: "windows", isRelease: true, env: map[string]string{"ChocolateyInstall": `D:\tools\choco`}, want: channel.Chocolatey},
		{name: "chocolatey default path mixed case", path: `C:\ProgramData\Chocolatey\Lib\m10rten-canary\tools\canary.exe`, goos: "windows", isRelease: true, want: channel.Chocolatey},
		{name: "chocolatey default path forward slashes", path: "C:/ProgramData/chocolatey/lib/m10rten-canary/tools/canary.exe", goos: "windows", isRelease: true, want: channel.Chocolatey},
		{name: "chocolatey env var empty and path elsewhere", path: `C:\Tools\canary.exe`, goos: "windows", isRelease: true, env: map[string]string{"ChocolateyInstall": ""}, want: channel.Unknown},
		{name: "chocolatey env var must match a whole segment", path: `C:\ProgramData\chocolatey-other\canary.exe`, goos: "windows", isRelease: true, env: chocoEnv, want: channel.Unknown},
		{name: "install.sh /usr/local/bin", path: "/usr/local/bin/canary", goos: "darwin", isRelease: true, want: channel.InstallSh},
		{name: "install.sh ~/.local/bin", path: "/home/me/.local/bin/canary", goos: "linux", isRelease: true, want: channel.InstallSh},
		{name: "dev build outside managed paths", path: "/usr/local/bin/canary", goos: "linux", isRelease: false, want: channel.Unknown},
		{name: "dev build still detects homebrew", path: "/opt/homebrew/Cellar/canary/dev/bin/canary", goos: "darwin", isRelease: false, want: channel.Homebrew},
		{name: "windows non-managed path", path: `C:\Users\me\bin\canary.exe`, goos: "windows", isRelease: true, want: channel.Unknown},
		{name: "directory merely named like a marker", path: "/home/me/cellarage/canary", goos: "linux", isRelease: true, want: channel.InstallSh},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if got := channel.Detect(tc.path, tc.goos, tc.isRelease, testutil.EnvOf(tc.env)); got != tc.want {
				t.Errorf("Detect(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestUpgradeCommand(t *testing.T) {
	testcases := []struct {
		channel channel.Channel
		want    string
		wantOK  bool
	}{
		{channel.NPM, "npm install -g @m10rten/go-canary-cli@latest", true},
		{channel.Homebrew, "brew upgrade m10rten/tap/canary", true},
		{channel.Chocolatey, "choco upgrade m10rten-canary", true},
		{channel.InstallSh, "", false},
		{channel.Unknown, "", false},
	}

	for _, tc := range testcases {
		t.Run(string(tc.channel), func(t *testing.T) {
			got, ok := channel.UpgradeCommand(tc.channel)
			if got != tc.want || ok != tc.wantOK {
				t.Errorf("UpgradeCommand(%q) = (%q, %v), want (%q, %v)", tc.channel, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestUpgradeCommandFor(t *testing.T) {
	testcases := []struct {
		channel channel.Channel
		distTag string
		want    string
	}{
		{channel.NPM, "latest", "npm install -g @m10rten/go-canary-cli@latest"},
		{channel.NPM, "nightly", "npm install -g @m10rten/go-canary-cli@nightly"},
		{channel.NPM, "rc", "npm install -g @m10rten/go-canary-cli@rc"},
		{channel.Homebrew, "nightly", "brew upgrade m10rten/tap/canary"},
		{channel.Chocolatey, "beta", "choco upgrade m10rten-canary"},
		{channel.InstallSh, "nightly", ""},
	}

	for _, tc := range testcases {
		t.Run(string(tc.channel)+"@"+tc.distTag, func(t *testing.T) {
			if got, _ := channel.UpgradeCommandFor(tc.channel, tc.distTag); got != tc.want {
				t.Errorf("UpgradeCommandFor(%q, %q) = %q, want %q", tc.channel, tc.distTag, got, tc.want)
			}
		})
	}
}
