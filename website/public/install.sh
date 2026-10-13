#!/bin/sh
# Sentra Installer — https://trysentra.tech
# Usage: curl -fsSL https://trysentra.tech/install.sh | sh
#
# Downloads the latest Sentra binary from GitHub Releases
# and installs it to /usr/local/bin/sentra.

set -e

REPO="code-ninjax/Sentra"
BINARY="sentra-linux-amd64"
INSTALL_DIR="/usr/local/bin"

# Colors
GREEN='\033[0;32m'
CYAN='\033[0;36m'
DIM='\033[2m'
RESET='\033[0m'

echo ""
echo "${GREEN}  ┌─────────────────────────────────────────┐${RESET}"
echo "${GREEN}  │         Sentra Installer                 │${RESET}"
echo "${GREEN}  │   Containers, minus the daemon.          │${RESET}"
echo "${GREEN}  └─────────────────────────────────────────┘${RESET}"
echo ""

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$OS" in
  linux) ;;
  *)
    echo "Error: Sentra currently supports Linux only."
    echo "  Your OS: $OS"
    echo ""
    echo "Follow progress on other platforms:"
    echo "  https://github.com/$REPO"
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) BINARY="sentra-linux-amd64" ;;
  aarch64|arm64) BINARY="sentra-linux-arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Get latest release tag
echo "${DIM}Fetching latest release...${RESET}"
RELEASE_URL="https://github.com/$REPO/releases/latest/download/$BINARY"

# Download
TMPFILE=$(mktemp)
echo "${CYAN}Downloading $BINARY...${RESET}"

if command -v curl > /dev/null 2>&1; then
  HTTP_CODE=$(curl -fsSL -w '%{http_code}' -o "$TMPFILE" "$RELEASE_URL" 2>/dev/null || true)
elif command -v wget > /dev/null 2>&1; then
  wget -q -O "$TMPFILE" "$RELEASE_URL" 2>/dev/null && HTTP_CODE="200" || HTTP_CODE="404"
else
  echo "Error: curl or wget is required."
  rm -f "$TMPFILE"
  exit 1
fi

if [ "$HTTP_CODE" != "200" ] || [ ! -s "$TMPFILE" ]; then
  echo ""
  echo "Error: Could not download Sentra binary."
  echo ""
  echo "  This can happen if:"
  echo "  • There is no release published yet on GitHub"
  echo "  • Your architecture ($ARCH) does not have a prebuilt binary"
  echo ""
  echo "  You can build from source instead:"
  echo "    git clone https://github.com/$REPO.git"
  echo "    cd Sentra"
  echo "    go build -o sentra ./cmd/sentra"
  echo "    sudo mv sentra $INSTALL_DIR/sentra"
  echo ""
  rm -f "$TMPFILE"
  exit 1
fi

# Install
chmod +x "$TMPFILE"

if [ -w "$INSTALL_DIR" ]; then
  mv "$TMPFILE" "$INSTALL_DIR/sentra"
else
  echo "${DIM}Installing to $INSTALL_DIR (requires sudo)...${RESET}"
  sudo mv "$TMPFILE" "$INSTALL_DIR/sentra"
fi

# Verify
VERSION=$("$INSTALL_DIR/sentra" --version 2>/dev/null || echo "installed")

echo ""
echo "${GREEN}✓ Sentra installed successfully!${RESET}"
echo ""
echo "  Binary:   $INSTALL_DIR/sentra"
echo "  Version:  $VERSION"
echo ""
echo "  Get started:"
echo "    ${CYAN}sentra run alpine echo 'Hello from Sentra!'${RESET}"
echo ""
echo "  Docs:     https://trysentra.tech/#/docs/quickstart"
echo "  GitHub:   https://github.com/$REPO"
echo ""
