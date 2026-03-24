set -e

# klarity installer
# Usage: curl -sSL https://getklarity.dev/install.sh | sh

BINARY_NAME="klarity"
REPO="vishukamble/klarity"
INSTALL_DIR="/usr/local/bin"

RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}Installing ${BINARY_NAME}...${NC}"

# ── Detect OS and architecture ────────────────────────────────────────────────

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"  # Darwin → darwin
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;   # x86_64 → amd64
    aarch64|arm64) ARCH="arm64" ;;
    *)
        echo -e "${RED}Unsupported architecture: ${ARCH}${NC}"
        exit 1
        ;;
esac

# ── Resolve latest release ───────────────────────────────────────────────────

LATEST_URL="https://api.github.com/repos/${REPO}/releases/latest"
VERSION="$(curl -sL -o /dev/null -w "%{url_effective}" \
  "https://github.com/${REPO}/releases/latest" \
  | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+')"

if [ -z "$VERSION" ] || [[ "$VERSION" != v* ]]; then
    echo -e "${RED}Failed to determine latest version.${NC}"
    echo -e "Visit https://github.com/${REPO}/releases and install manually."
    exit 1
fi

echo -e "  Version:  ${GREEN}${VERSION}${NC}"
echo -e "  OS:       ${OS}"
echo -e "  Arch:     ${ARCH}"

# ── Download and install ─────────────────────────────────────────────────────

TAR_FILE="${BINARY_NAME}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${TAR_FILE}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

echo -e "  Downloading ${DOWNLOAD_URL}..."

curl -sSL -o "${TMP_DIR}/${TAR_FILE}" "$DOWNLOAD_URL"

if [ ! -s "${TMP_DIR}/${TAR_FILE}" ]; then
    echo -e "${RED}Download failed or file is empty.${NC}"
    echo -e "Check available releases at: https://github.com/${REPO}/releases"
    exit 1
fi

tar -xzf "${TMP_DIR}/${TAR_FILE}" -C "$TMP_DIR"

if [ ! -f "${TMP_DIR}/${BINARY_NAME}" ]; then
    echo -e "${RED}Binary not found in archive. Contents:${NC}"
    ls -la "$TMP_DIR"
    exit 1
fi

chmod +x "${TMP_DIR}/${BINARY_NAME}"

# Install — use sudo if needed.
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
else
    echo -e "  Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${TMP_DIR}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
fi

# ── Verify ────────────────────────────────────────────────────────────────────

echo ""
echo -e "${GREEN}✅ ${BINARY_NAME} ${VERSION} installed to ${INSTALL_DIR}/${BINARY_NAME}${NC}"
echo ""
echo "Get started:"
echo "  ${BINARY_NAME} init      # configure environments"
echo "  ${BINARY_NAME}           # scan everything"
