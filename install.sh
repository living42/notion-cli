#!/usr/bin/env bash
#
# install.sh — one-click installer for notion-cli
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash -s -- --version v1.2.3
#   curl -fsSL https://raw.githubusercontent.com/living42/notion-cli/main/install.sh | bash -s -- --bin-dir ~/bin
#
# Detects your OS/architecture, downloads the matching pre-built binary from the
# latest GitHub release, verifies its checksum, and installs it.
#
set -euo pipefail

OWNER="living42"
REPO="notion-cli"
BIN_NAME="notion"

VERSION=""
BIN_DIR=""

# --- helpers -----------------------------------------------------------------

info()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn()  { printf '\033[1;33m==> WARNING:\033[0m %s\n' "$*" >&2; }
err()   { printf '\033[1;31m==> ERROR:\033[0m %s\n' "$*" >&2; exit 1; }

# Print the sha256 of a file, working on both Linux and macOS.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    err "Neither sha256sum nor shasum is available; cannot verify checksum."
  fi
}

# Download a URL to a file, using curl or wget.
download() {
  local url="$1" dest="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$dest"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
  else
    err "Neither curl nor wget is installed. Please install one and retry."
  fi
}

# --- args --------------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --version)
      [ $# -ge 2 ] || err "--version requires a value."
      VERSION="$2"; shift 2 ;;
    --bin-dir)
      [ $# -ge 2 ] || err "--bin-dir requires a value."
      BIN_DIR="$2"; shift 2 ;;
    -h|--help)
      cat <<EOF
Usage: install.sh [--version <tag>] [--bin-dir <dir>]

  --version <tag>   Version to install (e.g. v1.2.3). Defaults to the latest release.
  --bin-dir <dir>   Directory to install the binary into (e.g. /usr/local/bin).
                    Defaults to /usr/local/bin if writable, otherwise ~/.local/bin.
EOF
      exit 0 ;;
    *)
      err "Unknown option: $1" ;;
  esac
done

# --- platform detection ------------------------------------------------------

os_raw="$(uname -s)"
arch_raw="$(uname -m)"
case "$os_raw" in
  Darwin) os="darwin" ;;
  Linux)  os="linux"  ;;
  *) err "Unsupported operating system: $os_raw (expected Darwin or Linux)." ;;
esac
case "$arch_raw" in
  x86_64|amd64)  arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "Unsupported architecture: $arch_raw (expected x86_64/amd64 or arm64/aarch64)." ;;
esac

info "Detected platform: ${os}/${arch}"

# --- resolve version ---------------------------------------------------------

# Normalize: ensure a leading "v" so users can pass either "1.0.0" or "v1.0.0".
if [ -n "$VERSION" ]; then
  case "$VERSION" in
    v*) : ;;
    *)  VERSION="v${VERSION}" ;;
  esac
else
  info "Fetching latest release from GitHub..."
  api_json="$(download "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" -)"
  # Extract tag_name without relying on jq.
  VERSION="$(printf '%s' "$api_json" | grep -o '"tag_name":[[:space:]]*"[^"]*"' | head -n1 | sed -E 's/.*"tag_name":[[:space:]]*"([^"]*)".*/\1/')"
  [ -n "$VERSION" ] || err "Could not determine the latest release tag from GitHub."
fi

info "Installing notion-cli ${VERSION}"

# --- download ----------------------------------------------------------------

asset="notion_${VERSION}_${os}_${arch}.tar.gz"
download_url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${asset}"
checksums_url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/checksums.txt"

tmpdir="$(mktemp -d 2>/dev/null || mktemp -d -t notion-cli)"
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading ${asset}..."
download "$download_url" "${tmpdir}/${asset}"

# --- verify checksum ---------------------------------------------------------

info "Verifying checksum..."
download "$checksums_url" "${tmpdir}/checksums.txt" 2>/dev/null || true

if [ -f "${tmpdir}/checksums.txt" ]; then
  expected="$(grep -E "[[:space:]]+${asset}\$" "${tmpdir}/checksums.txt" | awk '{print $1}' | head -n1)"
  [ -n "$expected" ] || err "No checksum entry found for ${asset} in checksums.txt."
  actual="$(sha256_of "${tmpdir}/${asset}")"
  [ "$expected" = "$actual" ] || err "Checksum mismatch for ${asset}.
  expected: ${expected}
  actual:   ${actual}"
  info "Checksum OK."
else
  warn "checksums.txt not available for ${VERSION}; skipping verification."
fi

# --- extract & install -------------------------------------------------------

info "Extracting..."
tar -xzf "${tmpdir}/${asset}" -C "$tmpdir"
[ -f "${tmpdir}/${BIN_NAME}" ] || err "Archive did not contain a '${BIN_NAME}' binary."

if [ -n "$BIN_DIR" ]; then
  target_dir="$BIN_DIR"
elif [ -w "/usr/local/bin" ]; then
  target_dir="/usr/local/bin"
else
  target_dir="${HOME}/.local/bin"
fi

mkdir -p "$target_dir"
target="${target_dir}/${BIN_NAME}"

# Use cp (not mv) since $tmpdir may be on a different filesystem.
cp "${tmpdir}/${BIN_NAME}" "$target"
chmod +x "$target"

# --- done --------------------------------------------------------------------

info "Installed ${target}"

if ! command -v "$BIN_NAME" >/dev/null 2>&1; then
  case ":${PATH}:" in
    *":${target_dir}:"*) : ;;  # already on PATH
    *)
      printf '\n\033[1;33m==> NOTE:\033[0m %s is not on your PATH.\n' "$target_dir"
      printf 'Add it to your shell profile, e.g.:\n\n'
      printf '    echo "export PATH=\"%s:\$PATH\"" >> ~/.bashrc && source ~/.bashrc\n\n' "$target_dir"
      printf '(use ~/.zshrc instead of ~/.bashrc if you use zsh.)\n'
      ;;
  esac
fi

printf '\n\033[1;32m==>\033[0m notion-cli %s installed successfully!\n' "$VERSION"
printf 'Run \033[1mnotion configure\033[0m to set up your Notion integration token.\n'
