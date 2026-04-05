#!/usr/bin/env bash
# Gratis agent installer
# Usage: curl -fsSL https://sali.cloud/install-agent | bash
#   or:  bash install.sh --api gratis.example.com:9090 --token <token>
set -euo pipefail

BINARY_URL="${GRATIS_BINARY_URL:-https://github.com/salicloud/gratis/releases/latest/download/gratis-agent-linux-amd64}"
INSTALL_DIR="/usr/local/bin"
ENV_DIR="/etc/gratis"
SERVICE_FILE="/etc/systemd/system/gratis-agent.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[gratis]${NC} $*"; }
warn()  { echo -e "${YELLOW}[gratis]${NC} $*"; }
error() { echo -e "${RED}[gratis]${NC} $*" >&2; exit 1; }

[[ $EUID -eq 0 ]] || error "This script must be run as root."

# ─── Parse args ──────────────────────────────────────────────────────────────

API_ADDR=""
TOKEN=""

while [[ $# -gt 0 ]]; do
  case $1 in
    --api)   API_ADDR="$2"; shift 2 ;;
    --token) TOKEN="$2";    shift 2 ;;
    *) error "Unknown argument: $1" ;;
  esac
done

if [[ -z "$API_ADDR" ]]; then
  read -rp "Gratis API address (e.g. gratis.example.com:9090): " API_ADDR
fi
if [[ -z "$TOKEN" ]]; then
  read -rp "Provisioning token: " TOKEN
fi

[[ -n "$API_ADDR" ]] || error "API address is required."
[[ -n "$TOKEN"    ]] || error "Token is required."

# ─── Download binary ─────────────────────────────────────────────────────────

info "Downloading gratis-agent..."
curl -fsSL "$BINARY_URL" -o "$INSTALL_DIR/gratis-agent"
chmod +x "$INSTALL_DIR/gratis-agent"
info "Installed to $INSTALL_DIR/gratis-agent"

# ─── Write env file ──────────────────────────────────────────────────────────

mkdir -p "$ENV_DIR"
chmod 700 "$ENV_DIR"

cat > "$ENV_DIR/agent.env" <<EOF
GRATIS_API_ADDR=${API_ADDR}
GRATIS_TOKEN=${TOKEN}
EOF
chmod 600 "$ENV_DIR/agent.env"

info "Config written to $ENV_DIR/agent.env"

# ─── Install systemd service ─────────────────────────────────────────────────

curl -fsSL "https://raw.githubusercontent.com/salicloud/gratis/main/deploy/agent/gratis-agent.service" \
  -o "$SERVICE_FILE"

systemctl daemon-reload
systemctl enable gratis-agent
systemctl restart gratis-agent

info "gratis-agent service started."
systemctl status gratis-agent --no-pager

info ""
info "Done! This server should appear in your Gratis dashboard shortly."
