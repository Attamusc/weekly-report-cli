#!/usr/bin/env bash
# install.sh - Download and install weekly-report-cli binary
# Used by the GitHub Action to install the correct platform binary
set -euo pipefail

REPO="Attamusc/weekly-report-cli"
INSTALL_DIR="${RUNNER_TEMP:-/tmp}/weekly-report-cli-bin"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Normalize architecture names
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "::error::Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

# Normalize OS names
case "$OS" in
  darwin)  OS="darwin" ;;
  linux)   OS="linux" ;;
  mingw*|msys*|cygwin*)
    OS="windows"
    ;;
  *)
    echo "::error::Unsupported operating system: $OS"
    exit 1
    ;;
esac

echo "::group::Installing weekly-report-cli"
echo "Platform: ${OS}/${ARCH}"

# Resolve version
if [ "${VERSION:-latest}" = "latest" ]; then
  echo "Fetching latest version..."
  VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
  if [ -z "$VERSION" ]; then
    echo "::error::Failed to fetch latest version"
    exit 1
  fi
fi
echo "Version: ${VERSION}"

# Construct download URLs
ARCHIVE_EXT="tar.gz"
if [ "$OS" = "windows" ]; then
  ARCHIVE_EXT="zip"
fi

ARCHIVE_NAME="weekly-report-cli-${OS}-${ARCH}.${ARCHIVE_EXT}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${ARCHIVE_NAME}"
CHECKSUM_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

echo "Downloading from: ${DOWNLOAD_URL}"

# Create install directory
mkdir -p "${INSTALL_DIR}"
cd "${INSTALL_DIR}"

# Download archive and checksums
if ! curl -fsSL -o "${ARCHIVE_NAME}" "${DOWNLOAD_URL}"; then
  echo "::error::Failed to download ${ARCHIVE_NAME}"
  echo "::error::URL: ${DOWNLOAD_URL}"
  exit 1
fi

if ! curl -fsSL -o "checksums.txt" "${CHECKSUM_URL}"; then
  echo "::warning::Failed to download checksums.txt, skipping verification"
else
  # Verify checksum
  echo "Verifying checksum..."
  if command -v sha256sum &> /dev/null; then
    if ! grep "${ARCHIVE_NAME}" checksums.txt | sha256sum -c -; then
      echo "::error::Checksum verification failed"
      exit 1
    fi
  elif command -v shasum &> /dev/null; then
    if ! grep "${ARCHIVE_NAME}" checksums.txt | shasum -a 256 -c -; then
      echo "::error::Checksum verification failed"
      exit 1
    fi
  else
    echo "::warning::No checksum tool available, skipping verification"
  fi
fi

# Extract
echo "Extracting..."
if [ "$OS" = "windows" ]; then
  unzip -q "${ARCHIVE_NAME}"
else
  tar -xzf "${ARCHIVE_NAME}"
fi

# Find and make executable
BINARY_NAME="weekly-report-cli"
if [ "$OS" = "windows" ]; then
  # On Windows, the extracted binary has the full name
  BINARY_NAME="weekly-report-cli-windows-amd64.exe"
  if [ -f "$BINARY_NAME" ]; then
    mv "$BINARY_NAME" "weekly-report-cli.exe"
  fi
else
  # On Unix, rename the platform-specific binary
  PLATFORM_BINARY="weekly-report-cli-${OS}-${ARCH}"
  if [ -f "$PLATFORM_BINARY" ]; then
    mv "$PLATFORM_BINARY" "weekly-report-cli"
  fi
fi

chmod +x weekly-report-cli*

# Add to PATH
if [ -n "${GITHUB_PATH:-}" ]; then
  echo "${INSTALL_DIR}" >> "$GITHUB_PATH"
else
  export PATH="${INSTALL_DIR}:$PATH"
fi

echo "Installed weekly-report-cli ${VERSION} to ${INSTALL_DIR}"
echo "::endgroup::"
