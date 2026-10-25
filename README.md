# go-canary-cli

Canary app for testing Go CLI releases, upgrades, and multi-channel distribution.

## Goal

`go-canary-cli` is a throwaway test app for validating the full release lifecycle of a Go CLI before applying it to a real project. It does almost nothing on purpose: the point is the plumbing, not the features.

Installs also as `canary` cli.

Specifically, it exists to prove out:

- **Cross-platform builds:** producing static binaries for macOS, Linux, and Windows (amd64 and arm64) from a single CI pipeline.
- **Multi-channel distribution:** installing the same release via `install.sh`, npm, Homebrew, and Chocolatey, and confirming each channel delivers the correct binary.
- **Upgrades:** verifying that upgrading works through each channel, including a built-in self-update command, and that a channel never fights over the binary with another channel's install.
- **Release automation:** tagging a version and having everything build, sign, publish, and update without manual steps.
- **Developer-friendly CLI layer:** a simple, Node.js/TypeScript-style way (think commander.js or yargs) to define commands, flags, options, positional arguments, and environment variable fallbacks.

Success means a user on any supported platform can install it with one command, run `go-canary-cli --version` to see the exact version and build time, and upgrade to a newer release just as easily.

> ⚠️ This is a test project. Don't depend on it; it may be renamed, deprecated, or unpublished at any time.

## Verifying downloads

Every install channel picks the binary for your OS and architecture (macOS, Linux, or Windows; amd64 or arm64) and checks it against the SHA-256 checksum:

- **`install.sh`**: detects your OS and arch, downloads the matching archive and `checksums.txt`, and verifies the SHA-256 before installing.
- **npm**: installs the binary for your OS and arch and verifies its SHA-256 checksum.
- **Homebrew**: the formula selects the archive for your OS and arch, and Homebrew enforces the SHA-256 in the formula.
- **Chocolatey**: installs the Windows binary for your arch and checks its SHA-256 checksum.

To check a download by hand, get the archive for your platform and `checksums.txt` from the GitHub release page. Put both files in the same folder.

macOS:

```console
$ shasum -a 256 --ignore-missing -c checksums.txt
<archive>: OK
```

Linux:

```console
$ sha256sum --ignore-missing -c checksums.txt
<archive>: OK
```

Windows (PowerShell) prints `True` when the hash matches the line for that file in `checksums.txt`:

```powershell
(Get-FileHash <archive> -Algorithm SHA256).Hash -eq (Select-String -Path checksums.txt -Pattern '<archive>').Line.Split(' ')[0]
```

If the check fails, don't run the binary. Download it again.

## Commands

| Command          | Description                                                                                               |
| ---------------- | --------------------------------------------------------------------------------------------------------- |
| `canary`         | Prints a short greeting plus the current version. Useful as a smoke test after install.                   |
| `canary version` | Prints the version, commit, build time, and OS/arch. Also available as `canary --version`.                |
| `canary info`    | Shows where the binary lives and how it was installed (`install.sh`, npm, Homebrew, or Chocolatey).       |
| `canary check`   | Checks for a newer release without installing it. Exits `0` if up to date, `1` if an update is available. |
| `canary upgrade` | Upgrades to the latest release using the right method for how it was installed (see below).               |
| `canary help`    | Lists commands and global flags. Also available as `canary --help` or `canary <command> --help`.          |

### Global flags

| Flag         | Description                                                                 |
| ------------ | --------------------------------------------------------------------------- |
| `--json`     | Output machine-readable JSON (supported by `version`, `info`, and `check`). |
| `--no-color` | Disable colored output. Also respects the `NO_COLOR` environment variable.  |

### How `upgrade` behaves

`canary upgrade` respects the channel you installed from, so package managers stay in control of the binaries they manage:

- **`install.sh`**: downloads the latest release, verifies its checksum, and replaces the binary in place.
- **Homebrew**: prints `brew upgrade canary` instead of touching the binary.
- **npm**: prints `npm install -g <package>@latest` instead of touching the binary.
- **Chocolatey**: prints `choco upgrade canary` instead of touching the binary.

Pass `--version <tag>` to upgrade (or downgrade) to a specific release, e.g. `canary upgrade --version v0.2.0`. This only works for `install.sh` installs.

### Progress output

Waits of unknown length, like `canary check` looking for a release or `canary upgrade` resolving the latest version, show a spinner cycling through `⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏`.

Downloads during `canary upgrade` show a progress bar with percentage and bytes:

```console
⣿⣿⣿⣿⣿⣿⣀⣀⣀⣀ 60%  3.1/5.2 MB
```

When a step finishes, its spinner or bar is replaced by a single result line, such as `✓ Checksum verified`.

Progress output goes to stderr. It turns off automatically when stdout or stderr is not a terminal (pipes, CI) and when `--json` is set, so scripts get clean output. `--no-color` and `NO_COLOR` remove color but keep the glyphs.

### Example

```console
$ canary version
canary v0.1.3 (commit 4f2a9c1, built 2026-10-25T14:02:11Z, darwin/arm64)

$ canary check
Update available: v0.1.3 → v0.2.0

$ canary upgrade
Upgraded canary v0.1.3 → v0.2.0
```
