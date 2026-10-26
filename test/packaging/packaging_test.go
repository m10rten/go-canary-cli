package packaging_test

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/packaging"
)

const releaseBase = "https://github.com/m10rten/go-canary-cli/releases/download/v1.2.3/"

// digest returns a distinct valid SHA-256 hex digest for each n in 1..9.
func digest(n byte) string {
	return strings.Repeat(string('0'+n), 64)
}

func checksumsFor(skip string) string {
	files := []string{
		"canary_1.2.3_darwin_amd64.tar.gz",
		"canary_1.2.3_darwin_arm64.tar.gz",
		"canary_1.2.3_linux_amd64.tar.gz",
		"canary_1.2.3_linux_arm64.tar.gz",
		"canary_1.2.3_windows_amd64.zip",
		"canary_1.2.3_windows_arm64.zip",
	}
	var b strings.Builder
	for i, f := range files {
		if f != skip {
			b.WriteString(digest(byte(i+1)) + "  " + f + "\n")
		}
	}
	return b.String()
}

func platformEntry(base, file string, n byte) string {
	return "url \"" + base + file + "\"\n      sha256 \"" + digest(n) + "\"\n"
}

func TestHomebrew(t *testing.T) {
	const mirror = "http://127.0.0.1:8080/"

	testcases := []struct {
		name         string
		tag          string
		checksums    string
		baseURL      string
		wantContains []string
		wantErr      string
	}{
		{
			name:      "all four platform entries with their URL and hash",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: []string{
				platformEntry(releaseBase, "canary_1.2.3_darwin_arm64.tar.gz", 2),
				platformEntry(releaseBase, "canary_1.2.3_darwin_amd64.tar.gz", 1),
				platformEntry(releaseBase, "canary_1.2.3_linux_arm64.tar.gz", 4),
				platformEntry(releaseBase, "canary_1.2.3_linux_amd64.tar.gz", 3),
			},
		},
		{
			name:      "platform entries sit in the matching on_* blocks",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: []string{
				"on_macos do\n    on_arm do\n      url \"" + releaseBase + "canary_1.2.3_darwin_arm64",
				"on_macos do\n    on_arm do\n      url \"" + releaseBase + "canary_1.2.3_darwin_arm64.tar.gz\"\n      sha256 \"" + digest(2) + "\"\n    end\n    on_intel do\n      url \"" + releaseBase + "canary_1.2.3_darwin_amd64",
				"on_linux do\n    on_arm do\n      url \"" + releaseBase + "canary_1.2.3_linux_arm64",
			},
		},
		{
			name:      "version, binary and alias",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: []string{
				"class Canary < Formula\n",
				"  version \"1.2.3\"\n",
				"    bin.install \"canary\"\n",
				"    bin.install_symlink \"canary\" => \"go-canary-cli\"\n",
				"assert_match \"canary v#{version}\", shell_output(\"#{bin}/canary --version\")",
			},
		},
		{
			name:      "base URL replaces the release URL",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			baseURL:   mirror,
			wantContains: []string{
				platformEntry(mirror, "canary_1.2.3_darwin_arm64.tar.gz", 2),
				platformEntry(mirror, "canary_1.2.3_linux_amd64.tar.gz", 3),
			},
		},
		{name: "base URL with Ruby string syntax", tag: "v1.2.3", checksums: checksumsFor(""), baseURL: `http://x/#{system("id")}`, wantErr: "must not contain"},
		{name: "missing darwin archive", tag: "v1.2.3", checksums: checksumsFor("canary_1.2.3_darwin_arm64.tar.gz"), wantErr: "no checksum listed for canary_1.2.3_darwin_arm64.tar.gz"},
		{name: "missing linux archive", tag: "v1.2.3", checksums: checksumsFor("canary_1.2.3_linux_amd64.tar.gz"), wantErr: "no checksum listed for canary_1.2.3_linux_amd64.tar.gz"},
		{name: "checksums for another version", tag: "v1.2.4", checksums: checksumsFor(""), wantErr: "no checksum listed"},
		{name: "malformed checksums line", tag: "v1.2.3", checksums: "not a checksum line\n", wantErr: "malformed"},
		{name: "tag without v", tag: "1.2.3", checksums: checksumsFor(""), wantErr: "must start with v"},
		{name: "tag with two parts", tag: "v1.2", checksums: checksumsFor(""), wantErr: "not a valid semantic version"},
		{name: "empty tag", tag: "", checksums: checksumsFor(""), wantErr: "must start with v"},
		{name: "build metadata", tag: "v1.2.3+build.1", checksums: checksumsFor(""), wantErr: "build metadata"},
		{name: "prerelease is not carried", tag: "v1.3.0-rc.1", checksums: checksumsFor(""), wantErr: "stable releases only"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := packaging.Homebrew(tc.tag, []byte(tc.checksums), tc.baseURL)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
				}
				if got != "" {
					t.Errorf("formula = %q, want none on error", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formula lacks %q\n%s", want, got)
				}
			}
		})
	}
}

func chocolateyFile(t *testing.T, files []packaging.File, path string) string {
	t.Helper()
	for _, f := range files {
		if f.Path == path {
			return f.Content
		}
	}
	t.Fatalf("no file %q in %v", path, files)
	return ""
}

func TestChocolatey(t *testing.T) {
	const mirror = "file:///D:/a/dist"
	const nuspec = "m10rten-canary.nuspec"
	const install = "tools/chocolateyinstall.ps1"
	const uninstall = "tools/chocolateyuninstall.ps1"

	testcases := []struct {
		name         string
		tag          string
		checksums    string
		baseURL      string
		wantContains map[string][]string
		wantErr      string
	}{
		{
			name:      "install script holds both arch URLs and hashes",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: map[string][]string{install: {
				"{ 'AMD64', 'X64' -contains $_ } {\n    $url = '" + releaseBase + "canary_1.2.3_windows_amd64.zip'\n    $checksum = '" + digest(5) + "'\n  }",
				"'ARM64' {\n    $url = '" + releaseBase + "canary_1.2.3_windows_arm64.zip'\n    $checksum = '" + digest(6) + "'\n  }",
				"-ChecksumType 'sha256'",
				"default { throw",
			}},
		},
		{
			name:      "install script detects the architecture from the environment and adds the alias",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: map[string][]string{install: {
				"$env:PROCESSOR_ARCHITEW6432",
				"$env:PROCESSOR_ARCHITECTURE",
				"[System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture",
				"Install-ChocolateyZipPackage -PackageName $env:ChocolateyPackageName",
				"Install-BinFile -Name 'go-canary-cli' -Path (Join-Path $toolsDir 'canary.exe')",
			}},
		},
		{
			name:      "uninstall script removes the alias",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: map[string][]string{uninstall: {
				"Uninstall-BinFile -Name 'go-canary-cli' -Path (Join-Path $toolsDir 'canary.exe')",
			}},
		},
		{
			name:      "nuspec has the required fields",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			wantContains: map[string][]string{nuspec: {
				"<id>m10rten-canary</id>",
				"<version>1.2.3</version>",
				"<authors>m10rten</authors>",
				"<projectUrl>https://github.com/m10rten/go-canary-cli</projectUrl>",
				"<projectSourceUrl>https://github.com/m10rten/go-canary-cli</projectSourceUrl>",
				"<docsUrl>https://github.com/m10rten/go-canary-cli/blob/v1.2.3/README.md</docsUrl>",
				"<bugTrackerUrl>https://github.com/m10rten/go-canary-cli/issues</bugTrackerUrl>",
				"<copyright>Copyright (c) 2026 M10rten</copyright>",
				"<licenseUrl>https://github.com/m10rten/go-canary-cli/blob/v1.2.3/LICENSE</licenseUrl>",
				"<packageSourceUrl>https://github.com/m10rten/go-canary-cli</packageSourceUrl>",
				"<releaseNotes>https://github.com/m10rten/go-canary-cli/releases/tag/v1.2.3</releaseNotes>",
				"<requireLicenseAcceptance>false</requireLicenseAcceptance>",
				"<description>",
				`<file src="tools\**" target="tools" />`,
			}},
		},
		{
			name:      "base URL replaces the release URL",
			tag:       "v1.2.3",
			checksums: checksumsFor(""),
			baseURL:   mirror + "/",
			wantContains: map[string][]string{install: {
				"$url = '" + mirror + "/canary_1.2.3_windows_amd64.zip'",
				"$url = '" + mirror + "/canary_1.2.3_windows_arm64.zip'",
			}},
		},
		{name: "base URL with a single quote", tag: "v1.2.3", checksums: checksumsFor(""), baseURL: "http://x/'; calc; '", wantErr: "must not contain"},
		{name: "base URL with a typographic single quote", tag: "v1.2.3", checksums: checksumsFor(""), baseURL: "http://x/\u2019; calc; \u2018", wantErr: "must not contain"},
		{name: "base URL with a newline", tag: "v1.2.3", checksums: checksumsFor(""), baseURL: "http://x/\ncalc", wantErr: "must not contain"},
		{name: "missing amd64 archive", tag: "v1.2.3", checksums: checksumsFor("canary_1.2.3_windows_amd64.zip"), wantErr: "no checksum listed for canary_1.2.3_windows_amd64.zip"},
		{name: "missing arm64 archive", tag: "v1.2.3", checksums: checksumsFor("canary_1.2.3_windows_arm64.zip"), wantErr: "no checksum listed for canary_1.2.3_windows_arm64.zip"},
		{name: "checksums for another version", tag: "v1.2.4", checksums: checksumsFor(""), wantErr: "no checksum listed"},
		{name: "tag without v", tag: "1.2.3", checksums: checksumsFor(""), wantErr: "must start with v"},
		{name: "build metadata", tag: "v1.2.3+build.1", checksums: checksumsFor(""), wantErr: "build metadata"},
		{name: "prerelease is not carried", tag: "v1.3.0-beta.1", checksums: checksumsFor(""), wantErr: "Chocolatey carries stable releases only"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			files, err := packaging.Chocolatey(tc.tag, []byte(tc.checksums), tc.baseURL)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %v, want one containing %q", err, tc.wantErr)
				}
				if files != nil {
					t.Errorf("files = %v, want none on error", files)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var spec struct {
				ID      string `xml:"metadata>id"`
				Version string `xml:"metadata>version"`
			}
			if err := xml.Unmarshal([]byte(chocolateyFile(t, files, nuspec)), &spec); err != nil {
				t.Fatalf("nuspec is not well-formed XML: %v", err)
			}
			if wantVersion := strings.TrimPrefix(tc.tag, "v"); spec.ID != "m10rten-canary" || spec.Version != wantVersion {
				t.Errorf("nuspec id, version = %q, %q, want m10rten-canary, %q", spec.ID, spec.Version, wantVersion)
			}
			for path, wants := range tc.wantContains {
				got := chocolateyFile(t, files, path)
				for _, want := range wants {
					if !strings.Contains(got, want) {
						t.Errorf("%s lacks %q\n%s", path, want, got)
					}
				}
			}
		})
	}
}

func TestRun(t *testing.T) {
	testcases := []struct {
		name        string
		args        func(dir string) []string
		checksums   string
		wantCode    int
		wantStderr  string
		wantWritten bool
	}{
		{
			name:        "writes the formula and the three chocolatey files",
			args:        func(dir string) []string { return []string{"v1.2.3", checksumsPath(dir), outPath(dir), ""} },
			checksums:   checksumsFor(""),
			wantWritten: true,
		},
		{
			name:       "rejected tag writes nothing",
			args:       func(dir string) []string { return []string{"v1.2", checksumsPath(dir), outPath(dir), ""} },
			checksums:  checksumsFor(""),
			wantCode:   1,
			wantStderr: "not a valid semantic version",
		},
		{
			name:       "missing archive writes nothing",
			args:       func(dir string) []string { return []string{"v1.2.3", checksumsPath(dir), outPath(dir), ""} },
			checksums:  checksumsFor("canary_1.2.3_linux_arm64.tar.gz"),
			wantCode:   1,
			wantStderr: "no checksum listed",
		},
		{
			name:       "base URL one package cannot hold writes nothing",
			args:       func(dir string) []string { return []string{"v1.2.3", checksumsPath(dir), outPath(dir), "http://x/'y"} },
			checksums:  checksumsFor(""),
			wantCode:   1,
			wantStderr: "must not contain",
		},
		{
			name: "unreadable checksums file",
			args: func(dir string) []string {
				return []string{"v1.2.3", filepath.Join(dir, "absent.txt"), outPath(dir), ""}
			},
			wantCode:   1,
			wantStderr: "absent.txt",
		},
		{
			name:       "wrong argument count",
			args:       func(string) []string { return []string{"v1.2.3"} },
			wantCode:   2,
			wantStderr: "takes 4 argument",
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.checksums != "" {
				if err := os.WriteFile(checksumsPath(dir), []byte(tc.checksums), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			code := packaging.Run(tc.args(dir), &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tc.wantStderr)
			}

			formula := filepath.Join(outPath(dir), "homebrew", "canary.rb")
			wantFiles := []string{
				formula,
				filepath.Join(outPath(dir), "chocolatey", "m10rten-canary.nuspec"),
				filepath.Join(outPath(dir), "chocolatey", "tools", "chocolateyinstall.ps1"),
				filepath.Join(outPath(dir), "chocolatey", "tools", "chocolateyuninstall.ps1"),
			}
			if tc.wantWritten {
				data, err := os.ReadFile(formula)
				if err != nil || !strings.Contains(string(data), "version \"1.2.3\"") {
					t.Errorf("formula = %q, %v, want one for 1.2.3", data, err)
				}
				for _, path := range wantFiles {
					if _, err := os.Stat(path); err != nil {
						t.Error(err)
					}
				}
				if got, want := strings.Split(strings.TrimSpace(stdout.String()), "\n"), wantFiles; !slices.Equal(got, want) {
					t.Errorf("stdout lines = %q, want %q", got, want)
				}
				return
			}
			for _, path := range wantFiles {
				if _, err := os.Stat(path); err == nil {
					t.Errorf("%s was written on failure", path)
				}
			}
		})
	}
}

func checksumsPath(dir string) string { return filepath.Join(dir, "checksums.txt") }
func outPath(dir string) string       { return filepath.Join(dir, "packages") }
