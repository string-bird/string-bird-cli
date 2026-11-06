#!/bin/sh
# Installs sb, the String Bird CLI, from GitHub releases.
#
#   curl -fsSL https://raw.githubusercontent.com/string-bird/string-bird-cli/main/install.sh | sh
#
# Environment:
#   SB_VERSION      release tag to install (default: the latest release), e.g. v0.1.0
#   SB_INSTALL_DIR  where to put sb (default: /usr/local/bin if writable, else ~/.local/bin)
#   SB_DOWNLOAD_BASE  releases URL (default: https://github.com/string-bird/string-bird-cli/releases)
#
# Linux, macOS and Windows (Git Bash, MSYS2, Cygwin); amd64 and arm64. The
# archive is checked against the release's checksums.txt before installing.
set -eu

base=${SB_DOWNLOAD_BASE:-https://github.com/string-bird/string-bird-cli/releases}
base=${base%/}

fail() {
	echo "install.sh: $*" >&2
	exit 1
}

need() {
	command -v "$1" >/dev/null 2>&1 || fail "$1 is required"
}

need curl
need uname

case $(uname -s) in
Linux) os=linux ;;
Darwin) os=darwin ;;
MINGW* | MSYS* | CYGWIN*) os=windows ;;
*) fail "unsupported OS: $(uname -s)" ;;
esac

case $(uname -m) in
x86_64 | amd64) arch=amd64 ;;
aarch64 | arm64) arch=arm64 ;;
*) fail "unsupported architecture: $(uname -m)" ;;
esac

tag=${SB_VERSION:-}
if [ -z "$tag" ]; then
	# The latest release redirects to …/releases/tag/<tag>; no API call needed.
	latest=$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$base/latest") ||
		fail "couldn't find the latest release at $base/latest"
	tag=${latest##*/}
	case $tag in
	v*) ;;
	*) fail "no release found (got $latest)" ;;
	esac
fi
case $tag in
v*) ;;
*) tag=v$tag ;;
esac
version=${tag#v}

if [ "$os" = windows ]; then
	ext=zip
	bin=sb.exe
	need unzip
else
	ext=tar.gz
	bin=sb
	need tar
fi
archive="sb_${version}_${os}_${arch}.${ext}"

dir=${SB_INSTALL_DIR:-}
if [ -z "$dir" ]; then
	if [ -w /usr/local/bin ]; then
		dir=/usr/local/bin
	else
		dir="$HOME/.local/bin"
	fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

echo "Downloading sb $tag ($os/$arch)"
curl -fsSL -o "$tmp/$archive" "$base/download/$tag/$archive" ||
	fail "download failed: $base/download/$tag/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/download/$tag/checksums.txt" ||
	fail "download failed: $base/download/$tag/checksums.txt"

want=$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")
[ -n "$want" ] || fail "$archive isn't in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
	got=$(sha256sum "$tmp/$archive" | awk '{ print $1 }')
elif command -v shasum >/dev/null 2>&1; then
	got=$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')
else
	fail "sha256sum or shasum is required"
fi
[ "$want" = "$got" ] || fail "checksum mismatch for $archive: want $want, got $got"

if [ "$ext" = zip ]; then
	unzip -q "$tmp/$archive" "$bin" -d "$tmp"
else
	tar -xzf "$tmp/$archive" -C "$tmp" "$bin"
fi

mkdir -p "$dir"
mv "$tmp/$bin" "$dir/$bin"
chmod +x "$dir/$bin"
echo "Installed sb $tag to $dir/$bin"

case ":$PATH:" in
*":$dir:"*) ;;
*) echo "Add $dir to your PATH to run sb." ;;
esac
