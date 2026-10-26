// Package packaging renders the files that package managers need for a
// release from its tag and checksums.txt.
package packaging

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/m10rten/go-canary-cli/internal/channel"
	"github.com/m10rten/go-canary-cli/internal/release"
)

const usage = `usage:
  packaging TAG CHECKSUMS OUT_DIR BASE_URL   render the packages for TAG into OUT_DIR
                                             BASE_URL may be empty, it replaces the release download URL (tests only)`

//go:embed homebrew.rb.tmpl
var homebrewTemplate string

//go:embed chocolatey.nuspec.tmpl
var chocolateyNuspecTemplate string

//go:embed chocolateyinstall.ps1.tmpl
var chocolateyInstallTemplate string

//go:embed chocolateyuninstall.ps1.tmpl
var chocolateyUninstallTemplate string

var (
	homebrew            = template.Must(template.New("homebrew").Parse(homebrewTemplate))
	chocolateyNuspec    = template.Must(template.New("nuspec").Parse(chocolateyNuspecTemplate))
	chocolateyInstall   = template.Must(template.New("install").Parse(chocolateyInstallTemplate))
	chocolateyUninstall = template.Must(template.New("uninstall").Parse(chocolateyUninstallTemplate))
)

// File is a rendered file. Path is relative to the output directory and uses
// forward slashes.
type File struct {
	Path    string
	Content string
}

type asset struct {
	URL    string
	SHA256 string
}

type chocolateyData struct {
	ID, Tag, Version string
	Amd64, Arm64     asset
}

type homebrewData struct {
	Version                                          string
	DarwinArm64, DarwinAmd64, LinuxArm64, LinuxAmd64 asset
}

// Homebrew renders the formula for a stable tag. Each platform entry points at
// the release archive and carries its hash from checksums, the contents of a
// checksums.txt. A non-empty baseURL replaces the release download URL.
func Homebrew(tag string, checksums []byte, baseURL string) (string, error) {
	if err := requireStable(tag, "Homebrew"); err != nil {
		return "", err
	}

	if strings.ContainsAny(baseURL, `"\`) || strings.Contains(baseURL, "#{") {
		return "", fmt.Errorf("base URL %q must not contain a double quote, a backslash or #{ (it is written into a Ruby string)", baseURL)
	}

	data := homebrewData{Version: strings.TrimPrefix(tag, "v")}
	for goos, arches := range map[string]map[string]*asset{
		"darwin": {"arm64": &data.DarwinArm64, "amd64": &data.DarwinAmd64},
		"linux":  {"arm64": &data.LinuxArm64, "amd64": &data.LinuxAmd64},
	} {
		for goarch, entry := range arches {
			name := release.ArchiveName(tag, goos, goarch)
			sum, err := release.LookupChecksum(checksums, name)
			if err != nil {
				return "", err
			}
			*entry = asset{URL: assetURL(tag, name, baseURL), SHA256: sum}
		}
	}

	var out strings.Builder
	if err := homebrew.Execute(&out, data); err != nil {
		return "", fmt.Errorf("render formula: %w", err)
	}
	return out.String(), nil
}

// Chocolatey renders the package for a stable tag: the nuspec and the install
// and uninstall scripts. The install script carries the hash and URL of each
// Windows archive from checksums. A non-empty baseURL replaces the release
// download URL.
func Chocolatey(tag string, checksums []byte, baseURL string) ([]File, error) {
	if err := requireStable(tag, "Chocolatey"); err != nil {
		return nil, err
	}

	// The URL is written into PowerShell single-quoted strings, where the ASCII
	// and the typographic single quotes all end the string.
	if strings.ContainsAny(baseURL, "'\u2018\u2019\u201a\u201b\r\n") {
		return nil, fmt.Errorf("base URL %q must not contain a single quote or a line break (it is written into a PowerShell string)", baseURL)
	}

	data := chocolateyData{ID: channel.ChocolateyPackage, Tag: tag, Version: strings.TrimPrefix(tag, "v")}
	for goarch, entry := range map[string]*asset{"amd64": &data.Amd64, "arm64": &data.Arm64} {
		name := release.ArchiveName(tag, "windows", goarch)
		sum, err := release.LookupChecksum(checksums, name)
		if err != nil {
			return nil, err
		}
		*entry = asset{URL: assetURL(tag, name, baseURL), SHA256: sum}
	}

	var files []File
	for _, f := range []struct {
		path string
		tmpl *template.Template
	}{
		{channel.ChocolateyPackage + ".nuspec", chocolateyNuspec},
		{"tools/chocolateyinstall.ps1", chocolateyInstall},
		{"tools/chocolateyuninstall.ps1", chocolateyUninstall},
	} {
		var out strings.Builder
		if err := f.tmpl.Execute(&out, data); err != nil {
			return nil, fmt.Errorf("render %s: %w", f.path, err)
		}
		files = append(files, File{Path: f.path, Content: out.String()})
	}
	return files, nil
}

func requireStable(tag, manager string) error {
	track, err := release.ValidatePushedTag(tag)
	if err != nil {
		return err
	}
	if track != release.Stable {
		return fmt.Errorf("tag %q is a %s release: %s carries stable releases only", tag, track, manager)
	}
	return nil
}

func assetURL(tag, name, baseURL string) string {
	if baseURL == "" {
		return release.DownloadURL(tag, name)
	}
	return strings.TrimRight(baseURL, "/") + "/" + name
}

// Run renders the packages for a tag and returns the process exit code: 0 on
// success, 1 on a rejected input and 2 on a usage error. Nothing is written
// when rendering fails.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 4 {
		fmt.Fprintf(stderr, "packaging: takes 4 argument(s)\n%s\n", usage)
		return 2
	}
	tag, checksumsPath, outDir, baseURL := args[0], args[1], args[2], args[3]

	checksums, err := os.ReadFile(checksumsPath)
	if err != nil {
		fmt.Fprintf(stderr, "packaging: %v\n", err)
		return 1
	}
	formula, err := Homebrew(tag, checksums, baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "packaging: %v\n", err)
		return 1
	}
	chocolatey, err := Chocolatey(tag, checksums, baseURL)
	if err != nil {
		fmt.Fprintf(stderr, "packaging: %v\n", err)
		return 1
	}

	files := []File{{Path: "homebrew/canary.rb", Content: formula}}
	for _, f := range chocolatey {
		files = append(files, File{Path: "chocolatey/" + f.Path, Content: f.Content})
	}
	for _, f := range files {
		path := filepath.Join(outDir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(stderr, "packaging: %v\n", err)
			return 1
		}
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			fmt.Fprintf(stderr, "packaging: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, path)
	}
	return 0
}
