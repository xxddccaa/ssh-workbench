#!/bin/sh
set -eu

REPO="${SSHWB_REPO:-xxddccaa/ssh-workbench}"
VERSION="${SSHWB_VERSION:-}"
INSTALL_DIR="${SSHWB_DIR:-}"

usage() {
  cat <<'EOF'
Usage: install.sh [--version vX.Y.Z] [--dir /path/to/bin]

Installs or upgrades sshworkbench from GitHub Releases.
Environment variables:
  SSHWB_REPO     GitHub repo (default: xxddccaa/ssh-workbench)
  SSHWB_VERSION  Specific release tag (default: latest)
  SSHWB_DIR      Install directory (default: /usr/local/bin or ~/.local/bin)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      VERSION="${2:-}"
      shift 2
      ;;
    --dir)
      INSTALL_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Error: required command '$1' not found." >&2
    exit 1
  fi
}

need_cmd uname
need_cmd tar
need_cmd mktemp

fetch_text() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    echo "Error: curl or wget is required." >&2
    exit 1
  fi
}

fetch_file() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    echo "Error: curl or wget is required." >&2
    exit 1
  fi
}

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  darwin|linux) ;;
  *)
    echo "Error: unsupported OS '$OS'. Download Windows zip manually from GitHub Releases." >&2
    exit 1
    ;;
esac

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: unsupported architecture '$ARCH_RAW'." >&2
    exit 1
    ;;
esac

if [ -z "$VERSION" ]; then
  API_URL="https://api.github.com/repos/$REPO/releases/latest"
  TAG="$(fetch_text "$API_URL" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
  if [ -z "$TAG" ]; then
    echo "Error: failed to resolve latest release tag from $API_URL" >&2
    exit 1
  fi
else
  TAG="$VERSION"
fi

VER="${TAG#v}"
ARCHIVE="sshworkbench_${VER}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$TAG"

if [ -z "$INSTALL_DIR" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    INSTALL_DIR="/usr/local/bin"
  elif [ "$(id -u)" = "0" ]; then
    INSTALL_DIR="/usr/local/bin"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT HUP INT TERM

echo "Downloading $ARCHIVE ($TAG)..."
fetch_file "$BASE_URL/$ARCHIVE" "$TMPDIR/$ARCHIVE"

if fetch_file "$BASE_URL/SHA256SUMS" "$TMPDIR/SHA256SUMS" 2>/dev/null; then
  EXPECTED="$(grep " $ARCHIVE\$" "$TMPDIR/SHA256SUMS" | awk '{print $1}' | head -n 1 || true)"
  if [ -n "$EXPECTED" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      ACTUAL="$(sha256sum "$TMPDIR/$ARCHIVE" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      ACTUAL="$(shasum -a 256 "$TMPDIR/$ARCHIVE" | awk '{print $1}')"
    else
      ACTUAL=""
    fi
    if [ -n "$ACTUAL" ] && [ "$ACTUAL" != "$EXPECTED" ]; then
      echo "Error: SHA-256 mismatch for $ARCHIVE (expected $EXPECTED, got $ACTUAL)" >&2
      exit 1
    fi
  fi
fi

tar -xzf "$TMPDIR/$ARCHIVE" -C "$TMPDIR"
BIN_SRC="$TMPDIR/sshworkbench_${VER}_${OS}_${ARCH}/sshworkbench"
if [ ! -f "$BIN_SRC" ]; then
  BIN_SRC="$(find "$TMPDIR" -maxdepth 3 -type f -name sshworkbench | head -n 1)"
fi
if [ ! -f "$BIN_SRC" ]; then
  echo "Error: sshworkbench binary not found in archive." >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
chmod +x "$BIN_SRC"
mv "$BIN_SRC" "$INSTALL_DIR/sshworkbench"

echo "Installed sshworkbench $VER to $INSTALL_DIR/sshworkbench"
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "Add $INSTALL_DIR to PATH: export PATH=\"$INSTALL_DIR:\$PATH\"" ;;
esac
echo "Run 'sshworkbench' to start (or 'sshworkbench --listen 0.0.0.0:7331' for LAN/server deployment)."
