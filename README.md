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

- **`install.sh`**: detects your OS and arch, downloads the matching archive and `checksums.txt`, and verifies the SHA-256 before installing. It installs the latest stable release by default. Set `CANARY_TRACK` to `nightly`, `alpha`, `beta` or `rc` to install the newest release on that track's path (see the table under "How `upgrade` behaves"), or `CANARY_VERSION` to pin a tag, which takes precedence. If the install directory is not on your `PATH`, it adds it to the profile of the shell named in `$SHELL`: `~/.zshrc` (`$ZDOTDIR/.zshrc` if `ZDOTDIR` is exported), `~/.config/fish/conf.d/canary.fish`, `~/.profile` for sh, dash, ksh and ash, and for bash `~/.bashrc` on Linux or your existing bash login file (else `~/.bash_profile`) on macOS; other shells only get a hint. Set `CANARY_NO_MODIFY_PATH=1` to skip the edit and only print the hint.
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

| Command          | Description                                                                                                             |
| ---------------- | ----------------------------------------------------------------------------------------------------------------------- |
| `canary`         | Prints a short greeting plus the current version. Useful as a smoke test after install.                                 |
| `canary version` | Prints the version, commit, build time, and OS/arch. Also available as `canary --version`.                              |
| `canary info`    | Shows where the binary lives and how it was installed (`install.sh`, npm, Homebrew, Chocolatey, or `unknown`).          |
| `canary check`   | Checks for a newer release on your track without installing it. Exits `0` if up to date, `1` if an update is available. |
| `canary upgrade` | Upgrades to the newest release on your track using the right method for how it was installed (see below).               |
| `canary help`    | Lists commands and global flags. Also available as `canary --help` or `canary <command> --help`.                        |

### Global flags

| Flag         | Description                                                                 |
| ------------ | --------------------------------------------------------------------------- |
| `--json`     | Output machine-readable JSON (supported by `version`, `info`, and `check`). |
| `--no-color` | Disable colored output. Also respects the `NO_COLOR` environment variable.  |

### Exit codes

- `0`: success, including `canary upgrade` when already up to date.
- `1`: only when `canary check` finds an update.
- `2`: any error, such as a bad flag or a failed upgrade.

### How `upgrade` behaves

`canary upgrade` respects the channel you installed from, so package managers stay in control of the binaries they manage:

- **`install.sh`**: downloads the newest release on your track, verifies its checksum, and replaces the binary in place.
- **Homebrew**: prints `brew upgrade m10rten/tap/canary` instead of touching the binary.
- **npm**: prints `npm install -g @m10rten/go-canary-cli@<dist-tag>` instead of touching the binary, where `<dist-tag>` is your track: `latest` for stable, otherwise `nightly`, `alpha`, `beta` or `rc`. It makes no network call.
- **Chocolatey**: prints `choco upgrade m10rten-canary` instead of touching the binary.
- **`unknown`**: exits 2 with `cannot upgrade: install channel is unknown; reinstall canary with install.sh`.

Your track comes from the version you run: no prerelease part means stable, and a `-nightly.`, `-alpha.`, `-beta.` or `-rc.` prerelease means that track. Any other prerelease (such as `v0.0.0-ci`) or `dev` counts as stable. `check` and `install.sh` upgrades pick the highest release by version in the track's path, and only when it is newer than the running one:

| Track   | Path                    |
| ------- | ----------------------- |
| stable  | stable                  |
| nightly | nightly, stable         |
| rc      | rc, stable              |
| beta    | beta, rc, stable        |
| alpha   | alpha, beta, rc, stable |

Stable builds ask GitHub for the latest release. Other tracks read the first page of the releases list. Homebrew and Chocolatey carry stable releases only.

A Windows binary never reports the `install.sh` channel, so one outside Chocolatey or npm gets the `unknown` reinstall hint. Only an `install.sh` install on an OS other than macOS, Linux, or Windows (such as FreeBSD) exits 2 with `self-upgrade is not supported on <os>`.

Pass `--version <tag>` to upgrade (or downgrade) to a specific release, e.g. `canary upgrade --version v0.2.0`. This only works for `install.sh` installs. On other channels, it exits 2 with an error that names the channel and its upgrade command, or the reinstall hint for `unknown`.

For `install.sh` installs, the result line goes to stdout:

- `Upgraded canary v0.1.3 → v0.2.0` when the target is newer.
- `Downgraded canary v0.2.0 → v0.1.3` when `--version` names an older release.
- `Already up to date (v0.2.0)` when the target matches the current version, or when the current version is newer than the newest release on its track and no `--version` is given.

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

## Development

Requirements: Go 1.24, `make`, `git` and `tar`. `make dist` also needs `zip` (for the Windows archives) and either `sha256sum` or `shasum`. The Makefile is written for a POSIX shell on macOS or Linux.

Run these from the repo root:

| Command      | Description                                                                         |
| ------------ | ----------------------------------------------------------------------------------- |
| `make build` | Builds `bin/canary` for the host OS and arch.                                       |
| `make test`  | Runs `go vet ./...` and `go test ./...`.                                            |
| `make dist`  | Builds all six OS/arch targets and writes archives plus `checksums.txt` to `dist/`. |
| `make clean` | Removes `bin/` and `dist/`.                                                         |

Run the build with `./bin/canary version`.

Tests live in `test/`, one directory per package they cover (`test/cli` covers `internal/cli`, and `test/app` covers `internal/app`). Source directories hold no `_test.go` files, and tests use only exported identifiers. Shared helpers live in `test/testutil`.

The version comes from `VERSION`, which defaults to `git describe --tags --always --dirty`. That default can look like `v0.1.3-2-g4f2a9c1-dirty`, which is not a plain release version. Pass `VERSION=` with a release tag such as `v0.1.3` when you need release behavior, such as `check` or `upgrade`:

```console
$ make build VERSION=v0.1.3
$ make dist VERSION=v0.1.3
```

Run `make clean` before `make dist` if `dist/` holds archives from another version.

The Makefile sets these variables in `internal/buildinfo` through `-ldflags -X`. A plain `go build` leaves them at their defaults.

- `Version`: the tag from `VERSION`. Default `dev`.
- `Commit`: short commit SHA. Default `none`.
- `Date`: UTC build time in RFC 3339. Default `unknown`.

Builds use `CGO_ENABLED=0` and `-trimpath`.

### Release contracts

- **Archives:** `canary_<version without v>_<goos>_<goarch>.tar.gz` for darwin and linux, `.zip` for windows. `goos` and `goarch` use Go names (`darwin`, `linux`, `windows`; `amd64`, `arm64`).
- **Archive layout:** the binary is at the archive root as `canary` (`canary.exe` on Windows).
- **Checksums:** one `checksums.txt` per release, covering all archives, in `sha256sum` format: `<sha256>  <archive name>`.
- **JSON keys:**
  - `version`: `version`, `commit`, `build_time`, `os`, `arch`.
  - `info`: `path`, `channel`, `version`. `channel` is one of `install.sh`, `npm`, `homebrew`, `chocolatey`, `unknown`.
  - `check`: `current`, `latest`, `update_available`, `track`. `track` is the track of the running version: `stable`, `nightly`, `alpha`, `beta` or `rc`. `latest` is the newest release on that track's path.
