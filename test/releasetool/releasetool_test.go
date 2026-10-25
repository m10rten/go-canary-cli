package releasetool_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/releasetool"
)

func TestRun(t *testing.T) {
	testcases := []struct {
		name       string
		args       []string
		stdin      string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{"classify stable", []string{"classify", "v1.2.3"}, "", 0, "track=stable\nprerelease=false\ndist_tag=latest\n", ""},
		{"classify rc", []string{"classify", "v1.3.0-rc.1"}, "", 0, "track=rc\nprerelease=true\ndist_tag=rc\n", ""},
		{"classify rejects a pushed nightly", []string{"classify", "v1.3.0-nightly.20261025"}, "", 1, "", "releasetool: tag \"v1.3.0-nightly.20261025\" is a nightly tag"},
		{"classify rejects rc.0", []string{"classify", "v1.3.0-rc.0"}, "", 1, "", "N of 1 or more"},
		{"classify without a tag", []string{"classify", ""}, "", 1, "", "must start with v"},
		{"classify with no argument", []string{"classify"}, "", 2, "", "takes 1 argument"},

		{"next-nightly with no releases", []string{"next-nightly", "20261025"}, "", 0, "tag=v0.0.1-nightly.20261025\nnewest=\nexists=false\n", ""},
		{"next-nightly bumps the highest stable", []string{"next-nightly", "20261025"}, "v1.2.3\n\n  v1.2.2  \nv1.2.4-nightly.20261024\n", 0, "tag=v1.2.4-nightly.20261025\nnewest=v1.2.4-nightly.20261024\nexists=false\n", ""},
		{"next-nightly reports today's tag", []string{"next-nightly", "20261025"}, "v1.2.3\nv1.2.4-nightly.20261025\n", 0, "tag=v1.2.4-nightly.20261025\nnewest=v1.2.4-nightly.20261025\nexists=true\n", ""},
		{"next-nightly monotonic guard", []string{"next-nightly", "20261025"}, "v1.2.3\nv1.2.4-nightly.20261026\n", 1, "", "lower than the existing nightly"},
		{"next-nightly bad date", []string{"next-nightly", "2026-10-25"}, "", 1, "", "YYYYMMDD"},

		{"prune keeps the newest", []string{"prune-nightlies", "2"}, "v1.2.4-nightly.20261003\nv1.2.3\nv1.2.4-nightly.20261001\nv1.2.4-nightly.20261002\n", 0, "v1.2.4-nightly.20261001\n", ""},
		{"prune with nothing to delete prints nothing", []string{"prune-nightlies", "14"}, "v1.2.4-nightly.20261001\n", 0, "", ""},
		{"prune rejects keep zero", []string{"prune-nightlies", "0"}, "", 1, "", "keep must be 1 or more"},
		{"prune rejects a non-number", []string{"prune-nightlies", "many"}, "", 1, "", "must be a number"},

		{"dist-tags moves tags to the stable", []string{"dist-tags"}, `{"versions":["1.2.3","1.2.4-nightly.20261024","1.2.4"],"dist-tags":{"latest":"1.2.4","nightly":"1.2.4-nightly.20261024","beta":"1.2.3"}}`, 0, "nightly 1.2.4\nbeta 1.2.4\n", ""},
		{"dist-tags re-run prints nothing", []string{"dist-tags"}, `{"versions":["1.2.3","1.2.4"],"dist-tags":{"latest":"1.2.4","nightly":"1.2.4"}}`, 0, "", ""},
		{"dist-tags with a single published version as a string", []string{"dist-tags"}, `{"versions":"1.2.4","dist-tags":{"latest":"1.2.4","nightly":"1.2.4"}}`, 0, "", ""},
		{"dist-tags rejects invalid JSON", []string{"dist-tags"}, `not json`, 1, "", "read npm view output"},
		{"dist-tags rejects a missing versions list", []string{"dist-tags"}, `{"dist-tags":{"latest":"1.2.4"}}`, 1, "", "no versions list"},
		{"dist-tags rejects null versions", []string{"dist-tags"}, `{"versions":null,"dist-tags":{"latest":"1.2.4"}}`, 1, "", "null versions"},
		{"dist-tags takes no argument", []string{"dist-tags", "x"}, "", 2, "", "takes 0 argument"},

		{"no command", nil, "", 2, "", "usage:"},
		{"unknown command", []string{"publish"}, "", 2, "", "unknown command"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := releasetool.Run(tc.args, strings.NewReader(tc.stdin), &stdout, &stderr)
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tc.wantCode, stderr.String())
			}
			if stdout.String() != tc.wantStdout {
				t.Errorf("stdout = %q, want %q", stdout.String(), tc.wantStdout)
			}
			if !strings.Contains(stderr.String(), tc.wantStderr) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), tc.wantStderr)
			}
		})
	}
}
