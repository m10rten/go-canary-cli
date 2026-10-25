package release_test

import (
	"testing"

	"github.com/m10rten/go-canary-cli/internal/release"
)

func TestArchiveName(t *testing.T) {
	testcases := []struct {
		tag, goos, goarch string
		want              string
	}{
		{"v0.1.3", "darwin", "amd64", "canary_0.1.3_darwin_amd64.tar.gz"},
		{"v0.1.3", "darwin", "arm64", "canary_0.1.3_darwin_arm64.tar.gz"},
		{"v0.1.3", "linux", "amd64", "canary_0.1.3_linux_amd64.tar.gz"},
		{"v0.1.3", "linux", "arm64", "canary_0.1.3_linux_arm64.tar.gz"},
		{"v0.1.3", "windows", "amd64", "canary_0.1.3_windows_amd64.zip"},
		{"v0.1.3", "windows", "arm64", "canary_0.1.3_windows_arm64.zip"},
		{"0.1.3", "linux", "amd64", "canary_0.1.3_linux_amd64.tar.gz"},
		{"v1.0.0-rc.1", "linux", "amd64", "canary_1.0.0-rc.1_linux_amd64.tar.gz"},
	}

	for _, tc := range testcases {
		t.Run(tc.want, func(t *testing.T) {
			if got := release.ArchiveName(tc.tag, tc.goos, tc.goarch); got != tc.want {
				t.Errorf("ArchiveName(%q, %q, %q) = %q, want %q", tc.tag, tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestDownloadURL(t *testing.T) {
	testcases := []struct {
		tag, asset, want string
	}{
		{"v0.1.3", "canary_0.1.3_linux_amd64.tar.gz", "https://github.com/m10rten/go-canary-cli/releases/download/v0.1.3/canary_0.1.3_linux_amd64.tar.gz"},
		{"v0.1.3", release.ChecksumsName, "https://github.com/m10rten/go-canary-cli/releases/download/v0.1.3/checksums.txt"},
	}

	for _, tc := range testcases {
		t.Run(tc.asset, func(t *testing.T) {
			if got := release.DownloadURL(tc.tag, tc.asset); got != tc.want {
				t.Errorf("DownloadURL(%q, %q) = %q, want %q", tc.tag, tc.asset, got, tc.want)
			}
		})
	}
}
