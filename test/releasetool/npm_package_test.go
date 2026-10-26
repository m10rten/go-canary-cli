package releasetool_test

import (
	"bytes"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/releasetool"
)

const goldenManifest = `{
  "name": "@m10rten/go-canary-cli",
  "version": "1.2.3",
  "description": "Canary CLI for testing release automation: downloads and verifies the canary binary for your platform from the GitHub release",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/m10rten/go-canary-cli.git"
  },
  "bin": {
    "canary": "launcher.js",
    "go-canary-cli": "launcher.js"
  },
  "scripts": {
    "postinstall": "node install.js"
  },
  "files": [
    "install.js",
    "launcher.js"
  ],
  "os": [
    "darwin",
    "linux",
    "win32"
  ],
  "cpu": [
    "x64",
    "arm64"
  ],
  "engines": {
    "node": ">=22"
  },
  "publishConfig": {
    "access": "public"
  }
}
`

func runNPMPackage(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = releasetool.Run(append([]string{"npm-package"}, args...), strings.NewReader(""), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestNPMPackageGolden(t *testing.T) {
	code, stdout, stderr := runNPMPackage("v1.2.3")
	if code != 0 {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	if stdout != goldenManifest {
		t.Errorf("manifest differs from the golden manifest:\n%s", stdout)
	}
}

func TestNPMPackageVersion(t *testing.T) {
	testcases := []struct {
		tag  string
		want string
	}{
		{"v1.2.3", "1.2.3"},
		{"v1.3.0-beta.1", "1.3.0-beta.1"},
		{"v1.2.4-nightly.20261025", "1.2.4-nightly.20261025"},
		{"v0.0.0", "0.0.0"},
		{"v0.0.0-ci", "0.0.0-ci"},
	}

	for _, tc := range testcases {
		t.Run(tc.tag, func(t *testing.T) {
			code, stdout, stderr := runNPMPackage(tc.tag)
			if code != 0 {
				t.Fatalf("exit code = %d, stderr %q", code, stderr)
			}
			var manifest struct {
				Version string `json:"version"`
			}
			if err := json.Unmarshal([]byte(stdout), &manifest); err != nil {
				t.Fatalf("manifest is not JSON: %v", err)
			}
			if manifest.Version != tc.want {
				t.Errorf("version = %q, want %q", manifest.Version, tc.want)
			}
		})
	}
}

func TestNPMPackageRejectsTags(t *testing.T) {
	testcases := []struct {
		name       string
		args       []string
		wantCode   int
		wantStderr string
	}{
		{"no v prefix", []string{"1.2.3"}, 1, "must start with v"},
		{"two parts", []string{"v1.2"}, 1, "not a valid semantic version"},
		{"build metadata", []string{"v1.2.3+build.1"}, 1, "must not have build metadata"},
		{"empty tag", []string{""}, 1, "must start with v"},
		{"missing argument", nil, 2, "takes 1 argument"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, stderr := runNPMPackage(tc.args...)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d", code, tc.wantCode)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want containing %q", stderr, tc.wantStderr)
			}
		})
	}
}

// The checked-in npm/ folder must hold exactly the files the manifest names,
// and no package.json (it is generated at publish time).
func TestNPMFolderMatchesManifest(t *testing.T) {
	_, stdout, _ := runNPMPackage("v1.2.3")
	var manifest struct {
		Bin     map[string]string `json:"bin"`
		Files   []string          `json:"files"`
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal([]byte(stdout), &manifest); err != nil {
		t.Fatalf("manifest is not JSON: %v", err)
	}

	want := slices.Clone(manifest.Files)
	for _, file := range manifest.Bin {
		want = append(want, file)
	}
	want = append(want, strings.TrimPrefix(manifest.Scripts["postinstall"], "node "))
	slices.Sort(want)
	want = slices.Compact(want)

	entries, err := os.ReadDir("../../npm")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	slices.Sort(got)

	if !slices.Equal(got, want) {
		t.Errorf("npm/ holds %v, the manifest names %v", got, want)
	}
}
