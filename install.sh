#!/bin/sh
# leroymerlin installer — downloads a prebuilt binary from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/seifreed/leroymerlin-cli/main/install.sh | sh
#
# Env overrides:
#   LEROYMERLIN_VERSION=v0.1.0      install a specific tag (default: latest release)
#   LEROYMERLIN_INSTALL_DIR=/path   install location (default: /usr/local/bin, else ~/.local/bin)
set -eu

REPO="seifreed/leroymerlin-cli"
BIN="leroymerlin"

info() { printf '\033[1;34m==>\033[0m %s\n' "$1" >&2; }
err() {
	printf '\033[1;31merror:\033[0m %s\n' "$1" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || err "curl is required"
command -v tar >/dev/null 2>&1 || err "tar is required"

# --- detect target ---
os=$(uname -s)
case "$os" in
Darwin) os=darwin ;;
Linux) os=linux ;;
*) err "unsupported OS '$os' — download a release asset manually for Windows" ;;
esac

arch=$(uname -m)
case "$arch" in
x86_64 | amd64) arch=amd64 ;;
arm64 | aarch64) arch=arm64 ;;
*) err "unsupported architecture '$arch'" ;;
esac

# --- resolve version (tag carries the leading v; ver is bare for the asset name) ---
tag="${LEROYMERLIN_VERSION:-}"
if [ -z "$tag" ]; then
	info "resolving latest release…"
	tag=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" |
		grep '"tag_name"' | head -n1 | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
	[ -n "$tag" ] || err "could not determine the latest version (set LEROYMERLIN_VERSION=vX.Y.Z)"
fi
case "$tag" in v*) ver="${tag#v}" ;; *) ver="$tag" tag="v$tag" ;; esac

asset="${BIN}_${ver}_${os}_${arch}.tar.gz"
base="https://github.com/${REPO}/releases/download/${tag}"

# --- download, verify, extract ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

info "downloading ${asset} (${tag})…"
curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}" || err "download failed: ${base}/${asset}"

# Every release publishes checksums.txt, so a missing one means something is
# wrong with the download — fail rather than install an unverified binary.
curl -fsSL "${base}/checksums.txt" -o "${tmp}/checksums.txt" 2>/dev/null ||
	err "could not fetch ${base}/checksums.txt — refusing to install unverified"

info "verifying checksum…"
(
	cd "$tmp"
	grep " ${asset}\$" checksums.txt >expected.txt 2>/dev/null || err "no checksum entry for ${asset}"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum -c expected.txt >/dev/null 2>&1 || err "checksum mismatch for ${asset}"
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 -c expected.txt >/dev/null 2>&1 || err "checksum mismatch for ${asset}"
	else
		err "no sha256 tool found (sha256sum or shasum) — cannot verify ${asset}"
	fi
)

tar -xzf "${tmp}/${asset}" -C "$tmp" || err "failed to extract ${asset}"
[ -f "${tmp}/${BIN}" ] || err "binary '${BIN}' not found inside ${asset}"
chmod +x "${tmp}/${BIN}"

# --- choose an install dir ---
dir="${LEROYMERLIN_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
	if [ -w /usr/local/bin ] 2>/dev/null; then
		dir=/usr/local/bin
	else
		dir="${HOME}/.local/bin"
	fi
fi
mkdir -p "$dir" || err "could not create install dir '$dir'"

if mv "${tmp}/${BIN}" "${dir}/${BIN}" 2>/dev/null; then
	:
elif command -v sudo >/dev/null 2>&1; then
	info "writing to ${dir} needs sudo…"
	sudo mv "${tmp}/${BIN}" "${dir}/${BIN}" || err "install to ${dir} failed"
else
	err "cannot write to ${dir} (set LEROYMERLIN_INSTALL_DIR to a writable path)"
fi

info "installed ${BIN} → ${dir}/${BIN}"
case ":${PATH}:" in
*":${dir}:"*) ;;
*) info "note: ${dir} is not on your PATH — add it to use '${BIN}' directly" ;;
esac
"${dir}/${BIN}" version >/dev/null 2>&1 && info "run: ${BIN} --help"
