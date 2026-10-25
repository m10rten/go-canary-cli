#!/bin/sh
# Installs canary, and its go-canary-cli alias, from a GitHub release.
#
#   CANARY_VERSION            release tag to install (default: latest release)
#   CANARY_TRACK              stable (default), nightly, alpha, beta or rc: installs the
#                             newest release on that track's upgrade path (nightly: nightly
#                             or stable, rc: rc or stable, beta: beta, rc or stable, alpha:
#                             alpha, beta, rc or stable). CANARY_VERSION takes precedence.
#   CANARY_INSTALL_DIR        install directory (default: ~/.local/bin)
#   CANARY_DOWNLOAD_BASE_URL  mirror for release assets, fetched as <base>/<tag>/<asset>.
#                             Requires CANARY_VERSION. Must start with https:// or file://
#                             (file:// needs curl). checksums.txt comes from the same
#                             base, so verification is only as trustworthy as the mirror.
#
# curl enforces https on redirects too. wget enforces https only on the start URL,
# not on redirects.

set -eu

REPO=m10rten/go-canary-cli
tmp=
staged=
proto='=https'

say() { printf '%s\n' "$*" >&2; }
die() {
  say "install.sh: error: $*"
  exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

cleanup() {
  if [ -n "$staged" ]; then rm -f "$staged"; fi
  if [ -n "$tmp" ]; then rm -rf "$tmp"; fi
}

download() {
  if have curl; then
    curl -fsSL --proto "$proto" --proto-redir '=https' --tlsv1.2 --connect-timeout 15 --retry 2 -o "$2" "$1" ||
      die "download failed: $1${3:+. $3}"
  else
    wget -q -T 15 --https-only -O "$2" "$1" || die "download failed: $1${3:+. $3}"
  fi
}

sha256_of() {
  if have sha256sum; then
    sha256sum "$1" | awk '{ print $1 }'
  else
    shasum -a 256 "$1" | awk '{ print $1 }'
  fi
}

detect_platform() {
  os=$(uname -s)
  arch=$(uname -m)
  case $os in
    Darwin) goos=darwin ;;
    Linux) goos=linux ;;
    MINGW* | MSYS* | CYGWIN*)
      die "Windows shells are not supported. Install with npm (npm install -g @m10rten/go-canary-cli) or Chocolatey (choco install m10rten-canary)."
      ;;
    *) die "unsupported operating system: $os" ;;
  esac
  case $arch in
    x86_64 | amd64) goarch=amd64 ;;
    arm64 | aarch64) goarch=arm64 ;;
    *) die "unsupported architecture: $arch" ;;
  esac
  # A shell running under Rosetta reports x86_64 on Apple Silicon.
  if [ "$goos" = darwin ] && [ "$goarch" = amd64 ] &&
    [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || true)" = 1 ]; then
    goarch=arm64
  fi
}

# Prints a path with the symlinks in its existing part resolved.
physical_path() {
  p=$1
  rest=
  while [ ! -d "$p" ]; do
    rest=/$(basename "$p")$rest
    p=$(dirname "$p")
  done
  printf '%s%s\n' "$(cd "$p" && pwd -P)" "$rest"
}

# Like physical_path, but also follows symlinks in the last component.
resolved_path() {
  p=$1
  hops=0
  while [ -L "$p" ] && [ "$hops" -le 20 ]; do
    hops=$((hops + 1))
    link=$(readlink "$p")
    case $link in
      /*) p=$link ;;
      *) p=$(dirname "$p")/$link ;;
    esac
  done
  physical_path "$p"
}

managed_path() {
  case "/$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')/" in
    */cellar/* | */caskroom/* | */node_modules/* | */chocolatey/*) return 0 ;;
  esac
  return 1
}

# Refuses a path, or a symlink chain starting at it, that resolves into a
# directory owned by Homebrew, npm or Chocolatey.
refuse_managed() {
  path=$1
  hops=0
  while :; do
    if managed_path "$(physical_path "$path")"; then
      die "refusing to install over $1: it resolves into a Homebrew, npm or Chocolatey install. Upgrade through that package manager, or choose another directory with CANARY_INSTALL_DIR."
    fi
    [ -L "$path" ] || return 0
    hops=$((hops + 1))
    [ "$hops" -le 20 ] || die "too many symlinks resolving $1"
    link=$(readlink "$path")
    case $link in
      /*) path=$link ;;
      *) path=$(dirname "$path")/$link ;;
    esac
  done
}

# Prints the highest release of the upgrade path $1 in the releases list $2, like
# release.HighestInPath. Drafts and tags that are not vX.Y.Z, vX.Y.Z-(alpha|beta|rc).N
# with N >= 1 or vX.Y.Z-nightly.YYYYMMDD are ignored.
# Newlines are dropped and each object member is put on its own line, so the layout of
# the JSON does not matter. Inside a string value every quote is escaped, so a
# "tag_name" or "draft" key at the start of a line is never part of a release body.
highest_in_path() {
  tr -d '\r\n' <"$2" | tr ',{' '\n\n' | awk -v path=" $1 " '
    function pad(s) {
      while (length(s) < 20) s = "0" s
      return s
    }
    function consider(t,   core, pre, i, v, p, name, rank, key) {
      if (t !~ /^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-(alpha|beta|rc)\.[1-9][0-9]*|-nightly\.[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9])?$/) return
      core = substr(t, 2)
      pre = ""
      i = index(core, "-")
      if (i) {
        pre = substr(core, i + 1)
        core = substr(core, 1, i - 1)
      }
      split(core, v, ".")
      split(pre, p, ".")
      name = pre == "" ? "stable" : p[1]
      if (!index(path, " " name " ")) return
      rank = name == "alpha" ? 1 : name == "beta" ? 2 : name == "rc" ? 3 : name == "nightly" ? 4 : 9
      key = pad(v[1]) pad(v[2]) pad(v[3]) rank pad(p[2])
      if (best == "" || key > bestkey) {
        best = t
        bestkey = key
      }
    }
    # A release is complete once both of its keys were seen, in either order.
    function flush() {
      if (hastag && draft != "t") consider(tag)
      hastag = 0
      tag = ""
      draft = ""
    }
    /^[ \t]*"tag_name"[ \t]*:/ {
      if (hastag) flush()
      hastag = 1
      tag = $0
      sub(/^[^:]*:[ \t]*"/, "", tag)
      sub(/".*/, "", tag)
      if (draft != "") flush()
    }
    /^[ \t]*"draft"[ \t]*:/ {
      if (draft != "") flush()
      draft = ($0 ~ /:[ \t]*true/) ? "t" : "f"
      if (hastag) flush()
    }
    END {
      flush()
      if (best != "") print best
    }
  '
}

resolve_tag() {
  track=${CANARY_TRACK:-stable}
  case $track in
    stable) track_path=stable ;;
    nightly) track_path='nightly stable' ;;
    rc) track_path='rc stable' ;;
    beta) track_path='beta rc stable' ;;
    alpha) track_path='alpha beta rc stable' ;;
    *) die "CANARY_TRACK must be one of stable, nightly, alpha, beta or rc, got: $track" ;;
  esac
  if [ -n "${CANARY_VERSION:-}" ]; then
    tag=$CANARY_VERSION
    printf '%s\n' "$tag" |
      grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$' ||
      die "CANARY_VERSION must be a release tag like v1.2.3, got: $tag"
    return 0
  fi
  [ -z "${CANARY_DOWNLOAD_BASE_URL:-}" ] ||
    die "CANARY_VERSION is required when CANARY_DOWNLOAD_BASE_URL is set"
  if [ "$track" != stable ]; then
    # per_page=100 on purpose: wider than the CLI's 30, so a busy track is less likely to fall off the page.
    download "https://api.github.com/repos/$REPO/releases?per_page=100" "$tmp/releases.json" \
      "This may be a GitHub API rate limit. Set CANARY_VERSION to install a specific tag."
    tag=$(highest_in_path "$track_path" "$tmp/releases.json") || die "could not read the releases list"
    [ -n "$tag" ] || die "no release found on the $track track. Set CANARY_VERSION to install a specific tag."
    return 0
  fi
  download "https://api.github.com/repos/$REPO/releases/latest" "$tmp/latest.json" \
    "This may be a GitHub API rate limit. Set CANARY_VERSION to install a specific tag."
  tag=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp/latest.json" | head -n 1)
  printf '%s\n' "$tag" |
    grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' ||
    die "could not determine the latest release (got '$tag'). Set CANARY_VERSION to install a specific tag."
}

verify_checksum() {
  want=$(tr -d '\r' <"$3" | awk -v name="$2" '$2 == name || $2 == "*" name { print $1 }' | tr 'A-F' 'a-f')
  case $want in
    "") die "checksums.txt has no line for $2. Nothing was installed." ;;
    *[!0-9a-f]*) die "checksums.txt has a duplicate or malformed line for $2. Nothing was installed." ;;
  esac
  [ "${#want}" -eq 64 ] || die "checksums.txt has a malformed line for $2. Nothing was installed."
  got=$(sha256_of "$1")
  [ "$got" = "$want" ] ||
    die "checksum mismatch for $2 (expected $want, got $got). Nothing was installed."
}

report() {
  case ":$PATH:" in
    *":$dir:"*)
      found=$(command -v canary 2>/dev/null || true)
      if [ -n "$found" ] && [ "$(resolved_path "$found")" != "$(physical_path "$dir/canary")" ]; then
        say "warning: $found comes first on PATH and will shadow the canary installed in $dir"
      fi
      ;;
    *) say "$dir is not on your PATH. Add it with: export PATH=\"$dir:\$PATH\"" ;;
  esac
}

main() {
  have curl || have wget || die "curl or wget is required"
  have curl || wget --help 2>&1 | grep -q -e --https-only ||
    die "this wget does not support --https-only. Install curl."
  have sha256sum || have shasum || die "sha256sum or shasum is required"
  have tar || die "tar is required"

  if [ -n "${CANARY_DOWNLOAD_BASE_URL:-}" ]; then
    case $CANARY_DOWNLOAD_BASE_URL in
      https://*) ;;
      file://*)
        have curl || die "CANARY_DOWNLOAD_BASE_URL with file:// requires curl; wget does not support file://"
        proto='=https,file'
        ;;
      *) die "CANARY_DOWNLOAD_BASE_URL must start with https:// or file://, got: $CANARY_DOWNLOAD_BASE_URL" ;;
    esac
    say "warning: CANARY_DOWNLOAD_BASE_URL is set, downloading from $CANARY_DOWNLOAD_BASE_URL instead of GitHub"
  fi

  detect_platform

  if [ -n "${CANARY_INSTALL_DIR:-}" ]; then
    dir=$CANARY_INSTALL_DIR
  else
    [ -n "${HOME:-}" ] || die "HOME is not set; set CANARY_INSTALL_DIR"
    dir=$HOME/.local/bin
  fi
  case $dir in
    /*) ;;
    *) dir=$PWD/$dir ;;
  esac
  dir=${dir%/}
  refuse_managed "$dir/canary"
  refuse_managed "$dir/go-canary-cli"

  trap cleanup EXIT
  trap 'exit 129' HUP
  trap 'exit 130' INT
  trap 'exit 143' TERM
  trap 'exit 141' PIPE
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/canary-install.XXXXXX") || die "cannot create a temporary directory"

  resolve_tag
  archive=canary_${tag#v}_${goos}_${goarch}.tar.gz
  base=${CANARY_DOWNLOAD_BASE_URL:-https://github.com/$REPO/releases/download}
  base=${base%/}

  say "Downloading canary $tag for $goos/$goarch"
  download "$base/$tag/$archive" "$tmp/$archive"
  download "$base/$tag/checksums.txt" "$tmp/checksums.txt"
  verify_checksum "$tmp/$archive" "$archive" "$tmp/checksums.txt"

  mkdir "$tmp/extract"
  tar -xzf "$tmp/$archive" -C "$tmp/extract" canary || die "could not extract canary from $archive"
  if ! [ -f "$tmp/extract/canary" ] || [ -L "$tmp/extract/canary" ]; then
    die "$archive does not hold a canary binary at its root"
  fi

  mkdir -p "$dir" || die "cannot create $dir"
  staged=$(mktemp "$dir/.canary.XXXXXX") || die "cannot write to $dir"
  cp "$tmp/extract/canary" "$staged"
  chmod 755 "$staged"
  # mv would move into the directory instead of replacing it.
  if [ -d "$dir/canary" ]; then
    die "$dir/canary is a directory or a symlink to one"
  fi
  mv -f "$staged" "$dir/canary"
  staged=
  staged=$(mktemp "$dir/.go-canary-cli.XXXXXX") || die "cannot write to $dir"
  ln -sf canary "$staged"
  # mv would move into a symlinked directory instead of replacing the link.
  if [ -d "$dir/go-canary-cli" ]; then
    [ -L "$dir/go-canary-cli" ] || die "$dir/go-canary-cli is a directory"
    rm -f "$dir/go-canary-cli"
  fi
  mv -f "$staged" "$dir/go-canary-cli"
  staged=

  printf 'Installed canary %s to %s (alias: go-canary-cli)\n' "$tag" "$dir"
  report
}

main "$@"
