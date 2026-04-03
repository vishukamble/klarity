#!/bin/sh
set -e

# klarity installer
# Usage:       curl -sSL https://getklarity.dev/install.sh | sh
# User-only:   curl -sSL https://getklarity.dev/install.sh | INSTALL_DIR=~/.local/bin sh

BINARY_NAME="klarity"
REPO="vishukamble/klarity"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

printf "${BLUE}Installing ${BINARY_NAME}...${NC}\n"

# ── Detect OS and architecture ────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)        ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *)
        printf "${RED}Unsupported architecture: ${ARCH}${NC}\n"
        exit 1
        ;;
esac

# ── Resolve latest release ────────────────────────────────────────────────────

VERSION="$(curl -sL -o /dev/null -w "%{url_effective}" \
  "https://github.com/${REPO}/releases/latest" \
  | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"

if [ -z "$VERSION" ] || [ "${VERSION#v}" = "$VERSION" ]; then
    printf "${RED}Failed to determine latest version.${NC}\n"
    printf "Visit https://github.com/${REPO}/releases and install manually.\n"
    exit 1
fi

printf "  Version:  ${GREEN}${VERSION}${NC}\n"
printf "  OS:       ${OS}\n"
printf "  Arch:     ${ARCH}\n"

# ── Download and install ──────────────────────────────────────────────────────

TAR_FILE="${BINARY_NAME}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TAR_FILE}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

printf "  Downloading ${DOWNLOAD_URL}...\n"

curl -sSL -o "${TMP_DIR}/${TAR_FILE}" "$DOWNLOAD_URL"

if [ ! -s "${TMP_DIR}/${TAR_FILE}" ]; then
    printf "${RED}Download failed or file is empty.${NC}\n"
    printf "Check available releases at: https://github.com/${REPO}/releases\n"
    exit 1
fi

tar -xzf "${TMP_DIR}/${TAR_FILE}" -C "$TMP_DIR"

if [ ! -f "${TMP_DIR}/${BINARY_NAME}" ]; then
    printf "${RED}Binary not found in archive. Contents:${NC}\n"
    ls -la "$TMP_DIR"
    exit 1
fi

chmod +x "${TMP_DIR}/${BINARY_NAME}"

# Install — explain sudo when required; support INSTALL_DIR override.
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    printf "\n  ⚠  Installing to ${INSTALL_DIR} requires sudo (system-wide install).\n"
    printf "     This allows all users on this machine to run klarity.\n"
    printf "     If you prefer a user-only install, press Ctrl+C and re-run with:\n"
    printf "     curl -sSL https://getklarity.dev/install.sh | INSTALL_DIR=~/.local/bin sh\n\n"
    sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# ── Verify ────────────────────────────────────────────────────────────────────

printf "\n${GREEN}✅ ${BINARY_NAME} ${VERSION} installed to ${INSTALL_DIR}/${BINARY_NAME}${NC}\n\n"

# Warn if a user-local install dir is not yet in PATH.
case "$INSTALL_DIR" in
    "$HOME/.local/bin"|"$HOME/bin")
        case ":$PATH:" in
            *":$INSTALL_DIR:"*) ;;
            *)
                printf "  ⚠  Add to PATH: export PATH=\"\$HOME/.local/bin:\$PATH\"\n"
                printf "     Add this line to your ~/.zshrc or ~/.bashrc\n\n"
                ;;
        esac
        ;;
esac

printf "Get started:\n"
printf "  ${BINARY_NAME} init      # configure environments\n"
printf "  ${BINARY_NAME}           # scan everything\n"
