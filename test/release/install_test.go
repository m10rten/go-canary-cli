package release_test

import (
	"archive/tar"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/m10rten/go-canary-cli/internal/release"
	"github.com/m10rten/go-canary-cli/test/testutil"
)

func TestExtractBinary(t *testing.T) {
	testcases := []struct {
		name    string
		archive []byte
		want    string
		wantErr string
	}{
		{name: "binary at the root", archive: testutil.TarGz(t, testutil.TarEntry{Name: "canary", Body: "new"}), want: "new"},
		{name: "dot-slash prefix", archive: testutil.TarGz(t, testutil.TarEntry{Name: "./canary", Body: "new"}), want: "new"},
		{name: "extra files around the binary", archive: testutil.TarGz(t,
			testutil.TarEntry{Name: "README.md", Body: "readme"},
			testutil.TarEntry{Name: "canary", Body: "new"},
			testutil.TarEntry{Name: "LICENSE", Body: "license"}), want: "new"},
		{name: "binary missing", archive: testutil.TarGz(t, testutil.TarEntry{Name: "README.md", Body: "x"}), wantErr: "no canary at its top level"},
		{name: "binary only in a subdirectory", archive: testutil.TarGz(t, testutil.TarEntry{Name: "bin/canary", Body: "x"}), wantErr: "no canary at its top level"},
		{name: "empty archive", archive: testutil.TarGz(t), wantErr: "no canary at its top level"},
		{name: "path traversal", archive: testutil.TarGz(t, testutil.TarEntry{Name: "../canary", Body: "evil"}), wantErr: "unsafe path"},
		{name: "nested path traversal", archive: testutil.TarGz(t, testutil.TarEntry{Name: "a/../../etc/passwd", Body: "evil"}), wantErr: "unsafe path"},
		{name: "absolute path", archive: testutil.TarGz(t, testutil.TarEntry{Name: "/etc/passwd", Body: "evil"}), wantErr: "unsafe path"},
		{name: "symlink named canary", archive: testutil.TarGz(t, testutil.TarEntry{Name: "canary", Typeflag: tar.TypeSymlink, Linkname: "/bin/sh"}), wantErr: "not a regular file"},
		{name: "hardlink named canary", archive: testutil.TarGz(t, testutil.TarEntry{Name: "canary", Typeflag: tar.TypeLink, Linkname: "/bin/sh"}), wantErr: "not a regular file"},
		{name: "directory named canary", archive: testutil.TarGz(t, testutil.TarEntry{Name: "canary", Typeflag: tar.TypeDir}), wantErr: "not a regular file"},
		{name: "not gzip", archive: []byte("plain text"), wantErr: "open archive"},
		{name: "truncated archive", archive: testutil.TarGz(t, testutil.TarEntry{Name: "canary", Body: strings.Repeat("x", 4096)})[:40], wantErr: "archive"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := release.ExtractBinary(tc.archive, release.BinaryName)

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExtractBinary: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("binary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReplaceBinary(t *testing.T) {
	testcases := []struct {
		name        string
		mode        os.FileMode
		readOnlyDir bool
		posixOnly   bool
		wantErr     string
	}{
		{name: "swap keeps executable mode", mode: 0o755},
		{name: "swap keeps restrictive mode", mode: 0o700, posixOnly: true},
		{name: "unwritable directory", mode: 0o755, readOnlyDir: true, wantErr: "elevated permissions"},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && (tc.posixOnly || tc.readOnlyDir) {
				t.Skip("POSIX permission bits are not supported on Windows")
			}
			if tc.readOnlyDir && os.Geteuid() == 0 {
				t.Skip("root ignores directory permissions")
			}
			dir := t.TempDir()
			target := filepath.Join(dir, "canary")
			if err := os.WriteFile(target, []byte("old"), tc.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(target, tc.mode); err != nil {
				t.Fatal(err)
			}
			if tc.readOnlyDir {
				if err := os.Chmod(dir, 0o555); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(dir, 0o755) })
			}

			err := release.ReplaceBinary(target, []byte("new"))

			got, readErr := os.ReadFile(target)
			if readErr != nil {
				t.Fatal(readErr)
			}
			entries, _ := os.ReadDir(dir)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want it to contain %q", err, tc.wantErr)
				}
				if string(got) != "old" {
					t.Errorf("target = %q, want it untouched", got)
				}
				if len(entries) != 1 {
					t.Errorf("directory has %d entries, want only the original", len(entries))
				}
				return
			}
			if err != nil {
				t.Fatalf("ReplaceBinary: %v", err)
			}
			if string(got) != "new" {
				t.Errorf("target = %q, want %q", got, "new")
			}
			if info, _ := os.Stat(target); runtime.GOOS != "windows" && info.Mode().Perm() != tc.mode {
				t.Errorf("mode = %v, want %v", info.Mode().Perm(), tc.mode)
			}
			if len(entries) != 1 {
				t.Errorf("directory has %d entries, want no temp file left", len(entries))
			}
		})
	}
}

func TestReplaceBinaryMissingTarget(t *testing.T) {
	dir := t.TempDir()

	err := release.ReplaceBinary(filepath.Join(dir, "canary"), []byte("new"))

	if err == nil {
		t.Fatal("want an error when the target does not exist")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("directory has %d entries, want it empty", len(entries))
	}
}
