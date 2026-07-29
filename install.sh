#!/bin/bash
# install.sh — zai2api installer for Linux/macOS
# Usage: curl -fsSL https://raw.githubusercontent.com/hooshidev3/zai2api/main/install.sh | bash
set -euo pipefail

REPO="hooshidev3/zai2api"
INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="zai2api"
DATA_DIR="${ZAI2API_DATA:-/var/lib/zai2api}"
CONFIG_DIR="${ZAI2API_CONFIG:-/etc/zai2api}"
INSTALL_SERVICE=false
INSTALL_AUTOSTART=false

# Parse args
while [[ $# -gt 0 ]]; do
    case $1 in
        --service) INSTALL_SERVICE=true; shift ;;
        --autostart) INSTALL_AUTOSTART=true; INSTALL_SERVICE=true; shift ;;
        --dir) INSTALL_DIR="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

echo "=== zai2api Installer ==="

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

echo "Platform: ${OS}/${ARCH}"

# Get latest release tag
echo "Fetching latest release..."
TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$TAG" ]; then
    echo "Could not determine latest release. Using 'latest' tag."
    TAG="latest"
fi
echo "Version: ${TAG}"

# Download URL
if [ "$TAG" = "latest" ]; then
    DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/zai2api-${OS}-${ARCH}.tar.gz"
else
    DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${TAG}/zai2api_${OS}_${ARCH}.tar.gz"
fi

# Download and extract
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "Downloading from: ${DOWNLOAD_URL}"
curl -fsSL "$DOWNLOAD_URL" -o "$TMPDIR/zai2api.tar.gz"
tar xzf "$TMPDIR/zai2api.tar.gz" -C "$TMPDIR"

# Install binary
echo "Installing to ${INSTALL_DIR}/"
sudo mkdir -p "$INSTALL_DIR"
sudo cp "$TMPDIR/zai2api-${OS}-${ARCH}" "$INSTALL_DIR/zai2api"
sudo chmod +x "$INSTALL_DIR/zai2api"

# Create data directory
sudo mkdir -p "$DATA_DIR"
sudo chown -R "$(whoami)" "$DATA_DIR"

# Create config
sudo mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_DIR/.env" ]; then
    sudo tee "$CONFIG_DIR/.env" > /dev/null <<EOF
GATEWAY_TOKEN=sk-merged-change-me
ACCOUNTS_DB=${DATA_DIR}/accounts.sqlite
GLM_CAPTCHA_DB=${DATA_DIR}/tokens.sqlite
ZAI_STRATEGY=round-robin
EOF
    sudo chown "$(whoami)" "$CONFIG_DIR/.env"
    echo "Config created at ${CONFIG_DIR}/.env (edit GATEWAY_TOKEN!)"
fi

# Install systemd service
if [ "$INSTALL_SERVICE" = "true" ] && [ "$OS" = "linux" ]; then
    echo "Installing systemd service..."
    sudo tee /etc/systemd/system/${SERVICE_NAME}.service > /dev/null <<EOF
[Unit]
Description=zai2api Unified AI Gateway
After=network.target

[Service]
Type=simple
User=$(whoami)
EnvironmentFile=${CONFIG_DIR}/.env
ExecStart=${INSTALL_DIR}/zai2api
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    if [ "$INSTALL_AUTOSTART" = "true" ]; then
        sudo systemctl enable ${SERVICE_NAME}
        echo "Autostart enabled."
    fi
    sudo systemctl start ${SERVICE_NAME}
    echo "Service started. Check: systemctl status ${SERVICE_NAME}"
fi

echo ""
echo "✅ Installation complete!"
echo "   Binary: ${INSTALL_DIR}/zai2api"
echo "   Data:   ${DATA_DIR}"
echo "   Config: ${CONFIG_DIR}/.env"
echo ""
echo "Next steps:"
echo "  1. Edit config: sudo nano ${CONFIG_DIR}/.env"
echo "  2. Start:       zai2api  (or: systemctl start ${SERVICE_NAME})"
echo "  3. Dashboard:   http://localhost:8080"
