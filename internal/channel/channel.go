package channel

import "strings"

// Channel is how the running binary was installed. The string values are a
// stable contract (the `channel` key of `canary info --json`).
type Channel string

const (
	InstallSh  Channel = "install.sh"
	NPM        Channel = "npm"
	Homebrew   Channel = "homebrew"
	Chocolatey Channel = "chocolatey"
	Unknown    Channel = "unknown"
)

// Distribution names, each defined once. Upgrade commands are built from them.
const (
	NPMPackage        = "@m10rten/go-canary-cli"
	HomebrewFormula   = "m10rten/tap/canary"
	ChocolateyPackage = "m10rten-canary"
)

// UpgradeCommand returns the command a package manager runs to upgrade, or
// false for channels that are not package-manager managed.
func UpgradeCommand(c Channel) (string, bool) {
	return UpgradeCommandFor(c, "latest")
}

// UpgradeCommandFor is UpgradeCommand for a build on the npm dist-tag distTag.
// Homebrew and Chocolatey only carry stable releases, so distTag does not
// change their commands.
func UpgradeCommandFor(c Channel, distTag string) (string, bool) {
	switch c {
	case NPM:
		return "npm install -g " + NPMPackage + "@" + distTag, true
	case Homebrew:
		return "brew upgrade " + HomebrewFormula, true
	case Chocolatey:
		return "choco upgrade " + ChocolateyPackage, true
	}
	return "", false
}

// Detect classifies an install from the symlink-resolved executable path. It
// accepts both path separators and ignores case, so Windows paths classify
// the same on any host. isRelease reports whether the build has a release
// version; only release builds on non-Windows hosts are install.sh.
func Detect(path, goos string, isRelease bool, lookupEnv func(string) (string, bool)) Channel {
	p := normalize(path)
	segments := strings.Split(p, "/")

	switch {
	case hasSegment(segments, "cellar") || hasSegment(segments, "caskroom"):
		return Homebrew
	case hasSegment(segments, "node_modules"):
		return NPM
	case underChocolateyInstall(p, lookupEnv) || strings.Contains(p, "/chocolatey/lib/"):
		return Chocolatey
	case isRelease && goos != "windows":
		return InstallSh
	}
	return Unknown
}

func normalize(path string) string {
	return strings.ToLower(strings.ReplaceAll(path, `\`, "/"))
}

func hasSegment(segments []string, name string) bool {
	for _, s := range segments {
		if s == name {
			return true
		}
	}
	return false
}

func underChocolateyInstall(p string, lookupEnv func(string) (string, bool)) bool {
	root, ok := lookupEnv("ChocolateyInstall")
	root = strings.TrimRight(normalize(root), "/")
	return ok && root != "" && strings.HasPrefix(p, root+"/")
}
