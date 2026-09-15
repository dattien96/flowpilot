#!/usr/bin/env bash
# FlowPilot Install Script for macOS and Linux
# Usage: curl -fsSL https://raw.githubusercontent.com/dattien96/flowpilot/main/scripts/install.sh | bash

set -euo pipefail

REPO="dattien96/flowpilot"
INSTALL_DIR="${HOME}/.flowpilot/bin"

echo "==> Detecting platform and architecture..."

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "${OS}" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)
    echo "Error: Unsupported operating system: ${OS}"
    exit 1
    ;;
esac

case "${ARCH}" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: ${ARCH}"
    exit 1
    ;;
esac

echo "==> Fetching latest release info from GitHub..."
LATEST_TAG=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || echo "")

if [ -z "${LATEST_TAG}" ]; then
  # Fallback to hardcoded or default if API rate-limited
  LATEST_TAG="v1.0.0"
fi

VERSION="${LATEST_TAG#v}"
ARCHIVE_NAME="flowpilot_${VERSION}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_TAG}/${ARCHIVE_NAME}"

echo "==> Downloading FlowPilot ${LATEST_TAG} (${OS}/${ARCH})..."
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

if ! curl -fSL "${DOWNLOAD_URL}" -o "${TMP_DIR}/${ARCHIVE_NAME}"; then
  echo "Error: Failed to download ${DOWNLOAD_URL}"
  echo "Check if a release matching your platform exists: https://github.com/${REPO}/releases"
  exit 1
fi

echo "==> Installing binary to ${INSTALL_DIR}..."
mkdir -p "${INSTALL_DIR}"
tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "${TMP_DIR}"
cp "${TMP_DIR}/flowpilot" "${INSTALL_DIR}/flowpilot"
chmod +x "${INSTALL_DIR}/flowpilot"

echo ""
echo "✅ FlowPilot installed successfully to ${INSTALL_DIR}/flowpilot"
echo ""

# Check PATH
if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
  echo "⚠️  ${INSTALL_DIR} is not in your PATH."
  echo "   Add it to your shell configuration file (~/.zshrc or ~/.bashrc):"
  echo ""
  echo "   export PATH=\"\${HOME}/.flowpilot/bin:\$PATH\""
  echo ""
fi

echo "Run 'flowpilot --help' or 'flowpilot chat .' to get started!"
