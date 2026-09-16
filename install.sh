#!/usr/bin/env sh
# Orchard install script
# Usage: curl -fsSL https://raw.githubusercontent.com/hcarminati/orchard/main/install.sh | sh
# Or with a specific version:
#   curl -fsSL https://raw.githubusercontent.com/hcarminati/orchard/main/install.sh | sh -s -- --version v0.7.0

set -e

REPO="hcarminati/orchard"
BINARY="orchard"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# --------------------------------------------------------------------------

log()  { printf '  \033[32m%s\033[0m %s\n' '•' "$*"; }
warn() { printf '  \033[33m%s\033[0m %s\n' '!' "$*"; }
die()  { printf '  \033[31m%s\033[0m %s\n' '✗' "$*" >&2; exit 1; }

# --------------------------------------------------------------------------

detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    linux)  ;;
    darwin) ;;
    *)      die "Unsupported OS: $OS (only Linux and macOS are supported)" ;;
  esac

  case "$ARCH" in
    x86_64)          ARCH="amd64" ;;
    aarch64 | arm64) ARCH="arm64" ;;
    *)               die "Unsupported architecture: $ARCH" ;;
  esac

  PLATFORM="${OS}_${ARCH}"
}

# --------------------------------------------------------------------------

fetch_latest_version() {
  if [ -n "$VERSION" ]; then
    return
  fi
  log "Fetching latest release…"
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')
  if [ -z "$VERSION" ]; then
    die "Could not determine latest release version. Pass --version explicitly."
  fi
}

# --------------------------------------------------------------------------

download_and_install() {
  TARBALL="${BINARY}_${VERSION#v}_${PLATFORM}.tar.gz"
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${TARBALL}"

  log "Downloading ${BINARY} ${VERSION} for ${PLATFORM}…"

  TMP=$(mktemp -d)
  trap 'rm -rf "$TMP"' EXIT

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$URL" -o "$TMP/${TARBALL}"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$TMP/${TARBALL}" "$URL"
  else
    die "Neither curl nor wget found. Install one and retry."
  fi

  tar -xzf "$TMP/${TARBALL}" -C "$TMP"
  chmod +x "$TMP/${BINARY}"

  # Verify checksum if checksums file is available.
  SUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"
  if curl -fsSL "$SUMS_URL" -o "$TMP/checksums.txt" 2>/dev/null; then
    EXPECTED=$(grep "${TARBALL}" "$TMP/checksums.txt" | awk '{print $1}')
    if [ -n "$EXPECTED" ]; then
      if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL=$(sha256sum "$TMP/${TARBALL}" | awk '{print $1}')
      elif command -v shasum >/dev/null 2>&1; then
        ACTUAL=$(shasum -a 256 "$TMP/${TARBALL}" | awk '{print $1}')
      fi
      if [ -n "$ACTUAL" ] && [ "$ACTUAL" != "$EXPECTED" ]; then
        die "Checksum mismatch! Expected $EXPECTED, got $ACTUAL. Aborting."
      fi
      log "Checksum verified."
    fi
  fi

  # Install to INSTALL_DIR, using sudo if needed.
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  else
    log "Installing to ${INSTALL_DIR} (requires sudo)…"
    sudo mv "$TMP/${BINARY}" "${INSTALL_DIR}/${BINARY}"
  fi

  log "Installed ${BINARY} ${VERSION} to ${INSTALL_DIR}/${BINARY}"
}

# --------------------------------------------------------------------------

verify_install() {
  if command -v "$BINARY" >/dev/null 2>&1; then
    INSTALLED_VERSION=$("$BINARY" --version 2>/dev/null || echo "unknown")
    log "Verified: $(command -v $BINARY)  ($INSTALLED_VERSION)"
  else
    warn "${BINARY} not found in PATH. You may need to add ${INSTALL_DIR} to your PATH:"
    printf '\n  export PATH="%s:$PATH"\n\n' "$INSTALL_DIR"
  fi
}

# --------------------------------------------------------------------------

main() {
  # Parse flags.
  while [ $# -gt 0 ]; do
    case "$1" in
      --version) VERSION="$2"; shift 2 ;;
      --dir)     INSTALL_DIR="$2"; shift 2 ;;
      -h|--help)
        printf 'Usage: install.sh [--version TAG] [--dir PATH]\n'
        printf '  --version  install a specific release tag (e.g. v0.7.0)\n'
        printf '  --dir      installation directory (default: /usr/local/bin)\n'
        exit 0
        ;;
      *) die "Unknown option: $1" ;;
    esac
  done

  detect_platform
  fetch_latest_version
  download_and_install
  verify_install

  printf '\n'
  log "Next: run \`orchard setup\` to wire up Claude Code hooks."
  printf '\n'
}

main "$@"
