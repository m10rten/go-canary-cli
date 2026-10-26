package releasetool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/m10rten/go-canary-cli/internal/channel"
	"github.com/m10rten/go-canary-cli/internal/release"
)

// npmNodeEngine is the oldest Node LTS major that is still maintained (22,
// until 2027-04-30). The publish job runs on Node 22.14 or later anyway.
const npmNodeEngine = ">=22"

// npmManifest is the published package.json. The field order is the output
// order, so the JSON is byte-stable. It deliberately has no dist-tag: the tag
// is only passed to `npm publish --tag`.
type npmManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	License     string `json:"license"`
	Repository  struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	} `json:"repository"`
	Bin struct {
		Canary      string `json:"canary"`
		GoCanaryCLI string `json:"go-canary-cli"`
	} `json:"bin"`
	Scripts struct {
		Postinstall string `json:"postinstall"`
	} `json:"scripts"`
	Files   []string `json:"files"`
	OS      []string `json:"os"`
	CPU     []string `json:"cpu"`
	Engines struct {
		Node string `json:"node"`
	} `json:"engines"`
	PublishConfig struct {
		Access string `json:"access"`
	} `json:"publishConfig"`
}

// npmPackage renders the package.json for tag. npm drops build metadata, and
// the installer derives the release tag from the package version, so a tag
// with build metadata is rejected.
func npmPackage(tag string) (string, error) {
	if !strings.HasPrefix(tag, "v") {
		return "", fmt.Errorf("tag %q must start with v", tag)
	}
	if _, ok := release.ParseVersion(tag); !ok {
		return "", fmt.Errorf("tag %q is not a valid semantic version such as v1.2.3", tag)
	}
	if strings.Contains(tag, "+") {
		return "", fmt.Errorf("tag %q must not have build metadata: npm drops it, so the package would point at the wrong release", tag)
	}

	m := npmManifest{
		Name:        channel.NPMPackage,
		Version:     strings.TrimPrefix(tag, "v"),
		Description: "Canary CLI for testing release automation: downloads and verifies the canary binary for your platform from the GitHub release",
		License:     "MIT",
		Files:       []string{"install.js", "launcher.js"},
		OS:          []string{"darwin", "linux", "win32"},
		CPU:         []string{"x64", "arm64"},
	}
	m.Repository.Type = "git"
	m.Repository.URL = "git+https://github.com/m10rten/go-canary-cli.git"
	m.Bin.Canary = "launcher.js"
	m.Bin.GoCanaryCLI = "launcher.js"
	m.Scripts.Postinstall = "node install.js"
	m.Engines.Node = npmNodeEngine
	m.PublishConfig.Access = "public"

	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return "", fmt.Errorf("render package.json: %w", err)
	}
	return out.String(), nil
}
