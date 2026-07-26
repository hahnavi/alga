#!/usr/bin/env bash
# Alga Agent installer — downloads the latest alga-agent release binary from
# GitHub and installs it to $HOME/.local/bin.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/hahnavi/alga/main/scripts/install-agent.sh | bash
#   # or: ./scripts/install-agent.sh [version]   (e.g. 0.0.1, defaults to latest)
set -euo pipefail

REPO="hahnavi/alga"
INSTALL_DIR="${HOME}/.local/bin"
BIN_NAME="alga-agent"

info() { printf '\033[1;34m==>\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || err "curl is required"

# --- Detect platform ---------------------------------------------------------
os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *) err "unsupported OS: $os (only linux and darwin binaries are published)" ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) err "unsupported architecture: $arch (only amd64 and arm64 binaries are published)" ;;
esac

# --- Resolve version ---------------------------------------------------------
# Agent releases are tagged agent-v<semver>; the repo also publishes app (v*)
# and chart (chart-v*) releases, so "latest" must be filtered by tag prefix.
version="${1:-}"
if [ -z "$version" ]; then
  info "Resolving latest agent release..."
  version="$(curl -fsSL --connect-timeout 10 --max-time 30 "https://api.github.com/repos/${REPO}/releases?per_page=100" \
    | grep -o '"tag_name": *"agent-v[^"]*"' \
    | sed 's/.*agent-v//; s/"$//' \
    | head -n1)"
  [ -n "$version" ] || err "could not find an agent-v* release for ${REPO}"
fi
version="${version#agent-v}"
version="${version#v}"

asset="${BIN_NAME}-${version}-${os}-${arch}"
base_url="https://github.com/${REPO}/releases/download/agent-v${version}"
info "Installing ${BIN_NAME} ${version} (${os}/${arch})"

# --- Download and verify -----------------------------------------------------
tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fSL --connect-timeout 10 --max-time 300 --progress-bar -o "${tmpdir}/${asset}" "${base_url}/${asset}" \
  || err "download failed: ${base_url}/${asset}"

checksums="checksums-agent-${version}.txt"
if curl -fsSL --connect-timeout 10 --max-time 30 -o "${tmpdir}/${checksums}" "${base_url}/${checksums}"; then
  if command -v sha256sum >/dev/null 2>&1; then
    (cd "$tmpdir" && grep " ${asset}\$" "$checksums" | sha256sum -c - >/dev/null) \
      || err "checksum verification failed for ${asset}"
    info "Checksum verified"
  elif command -v shasum >/dev/null 2>&1; then
    (cd "$tmpdir" && grep " ${asset}\$" "$checksums" | shasum -a 256 -c - >/dev/null) \
      || err "checksum verification failed for ${asset}"
    info "Checksum verified"
  else
    info "sha256sum/shasum not found; skipping checksum verification"
  fi
else
  info "Checksums file not found; skipping verification"
fi

# --- Install -----------------------------------------------------------------
mkdir -p "$INSTALL_DIR"
install -m 0755 "${tmpdir}/${asset}" "${INSTALL_DIR}/${BIN_NAME}"
info "Installed to ${INSTALL_DIR}/${BIN_NAME}"

# --- Ensure $HOME/.local/bin is on PATH --------------------------------------
path_line='export PATH="$HOME/.local/bin:$PATH"'
case ":$PATH:" in
  *":${INSTALL_DIR}:"*)
    ;;
  *)
    shell_name="$(basename "${SHELL:-}")"
    case "$shell_name" in
      bash) rc="${HOME}/.bashrc" ;;
      zsh)  rc="${HOME}/.zshrc" ;;
      *)    rc="" ;;
    esac
    if [ -n "$rc" ]; then
      if ! grep -qsF "$path_line" "$rc"; then
        printf '\n# Added by alga-agent installer\n%s\n' "$path_line" >> "$rc"
        info "Added ${INSTALL_DIR} to PATH in ${rc}"
      fi
      info "Restart your shell (or run: source ${rc}) to pick up the new PATH"
    else
      info "Your shell (${shell_name:-unknown}) is not bash or zsh."
      info "Add ${INSTALL_DIR} to your PATH manually, e.g.:"
      info "  ${path_line}"
    fi
    ;;
esac

echo
info "Done! Get started with the onboarding wizard:"
info "  ${BIN_NAME} setup"
