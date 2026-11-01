#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Server - Inicializador com Túnel e QR Code
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[0;32m"
YELLOW="\033[0;33m"
CYAN="\033[0;36m"
RED="\033[0;31m"
RESET="\033[0m"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Carregar variáveis de ambiente se existir .env
if [ -f "$SCRIPT_DIR/.env" ]; then
    set -a
    source "$SCRIPT_DIR/.env" 2>/dev/null || true
    set +a
fi

PORT="${PORT:-4040}"

is_port_in_use() {
    local p="$1"
    if which lsof >/dev/null 2>&1; then
        lsof -iTCP:"$p" -sTCP:LISTEN -P -n >/dev/null 2>&1
        return $?
    elif which nc >/dev/null 2>&1; then
        nc -z 127.0.0.1 "$p" 2>/dev/null
        return $?
    fi
    return 1
}

ORIGINAL_PORT="$PORT"
while is_port_in_use "$PORT"; do
    echo -e "${YELLOW}⚠️ Porta $PORT ocupada por outro processo. Procurando próxima porta livre...${RESET}"
    PORT=$((PORT + 1))
done

if [ "$PORT" -ne "$ORIGINAL_PORT" ]; then
    echo -e "${GREEN}✓ Porta livre selecionada automaticamente:${RESET} ${BOLD}${PORT}${RESET}"
fi
export PORT

# Localizar cloudflared
CLOUDFLARED="$SCRIPT_DIR/bin/cloudflared"
if [ ! -x "$CLOUDFLARED" ]; then
    CLOUDFLARED="$(which cloudflared 2>/dev/null || true)"
fi

TUNNEL_PID=""
SERVER_PID=""

cleanup() {
    echo -e "\n${YELLOW}Encerrando servidor e túnel...${RESET}"
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
        kill "$SERVER_PID" 2>/dev/null || true
    fi
    if [ -n "$TUNNEL_PID" ] && kill -0 "$TUNNEL_PID" 2>/dev/null; then
        kill "$TUNNEL_PID" 2>/dev/null || true
    fi
    exit 0
}

trap cleanup SIGINT SIGTERM EXIT

# ------------------------------------------------------------------------------
# Conectividade Remota: Quick Tunnel ou Domínio Próprio
# ------------------------------------------------------------------------------
if [ -n "$CLOUDFLARE_TUNNEL_TOKEN" ]; then
    echo -e "${CYAN}Conectando via túnel Cloudflare configurado...${RESET}"
    "$CLOUDFLARED" tunnel run --token "$CLOUDFLARE_TUNNEL_TOKEN" >/dev/null 2>&1 &
    TUNNEL_PID=$!
elif [ -n "$CLOUDFLARED" ] && [ -x "$CLOUDFLARED" ]; then
    echo -e "${CYAN}Iniciando Quick Tunnel gratuito Cloudflare (Zero-Config)...${RESET}"
    TUNNEL_LOG="/tmp/cf_quick_tunnel_${PORT}.log"
    rm -f "$TUNNEL_LOG"
    "$CLOUDFLARED" tunnel --url "http://127.0.0.1:${PORT}" --protocol http2 > "$TUNNEL_LOG" 2>&1 &
    TUNNEL_PID=$!

    echo -ne "Aguardando URL pública do túnel"
    for i in {1..20}; do
        sleep 1
        echo -ne "."
        DETECTED_URL="$(grep -o 'https://[-a-zA-Z0-9.]*\.trycloudflare\.com' "$TUNNEL_LOG" | head -n 1 || true)"
        if [ -n "$DETECTED_URL" ]; then
            PUBLIC_URL="$DETECTED_URL"
            export PUBLIC_URL
            break
        fi
    done
    echo ""

    if [ -n "$PUBLIC_URL" ]; then
        echo -e "${GREEN}✓ Túnel público estabelecido:${RESET} ${BOLD}${PUBLIC_URL}${RESET}"
    else
        echo -e "${YELLOW}⚠️ O túnel demorou a responder. O servidor iniciará apenas com os links locais/IP.${RESET}"
    fi
fi

# ------------------------------------------------------------------------------
# Iniciar Servidor Go
# ------------------------------------------------------------------------------
if [ ! -f "$SCRIPT_DIR/antigravity-server" ]; then
    OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
    ARCH="$(uname -m)"
    case "$ARCH" in
        x86_64|amd64) GO_ARCH="amd64" ;;
        aarch64|arm64) GO_ARCH="arm64" ;;
        *) GO_ARCH="amd64" ;;
    esac
    PREBUILT="$SCRIPT_DIR/bin/antigravity-server-${OS}-${GO_ARCH}"
    if [ -f "$PREBUILT" ]; then
        cp "$PREBUILT" "$SCRIPT_DIR/antigravity-server"
        chmod +x "$SCRIPT_DIR/antigravity-server"
    elif which go >/dev/null 2>&1; then
        echo -e "${CYAN}Compilando antigravity-server...${RESET}"
        go build -ldflags="-s -w" -o "$SCRIPT_DIR/antigravity-server" .
    else
        echo -e "${RED}❌ Binário antigravity-server não encontrado e Go não está instalado.${RESET}"
        echo "Por favor execute ./install.sh primeiro."
        exit 1
    fi
fi

"$SCRIPT_DIR/antigravity-server" &
SERVER_PID=$!

wait "$SERVER_PID"
