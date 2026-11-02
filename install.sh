#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Server - Instalador Inteligente Multiplataforma
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
RED="\033[31m"
GRAY="\033[90m"
RESET="\033[0m"

echo ""
echo -e "${CYAN}╭─────────────────────────────────────────────────────────────╮${RESET}"
echo -e "${CYAN}│${RESET}  ${BOLD}ANTIGRAVITY SERVER • INSTALADOR${RESET}                            ${CYAN}│${RESET}"
echo -e "${CYAN}│${RESET}  ${GRAY}WebuildSoft Engine • Configuração Automatizada             ${RESET}${CYAN}│${RESET}"
echo -e "${CYAN}╰─────────────────────────────────────────────────────────────╯${RESET}"
echo ""

# 1. Detectar Sistema Operacional e Arquitetura
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64|amd64)
        CLOUDFLARED_ARCH="amd64"
        GO_ARCH="amd64"
        ;;
    aarch64|arm64)
        CLOUDFLARED_ARCH="arm64"
        GO_ARCH="arm64"
        ;;
    *)
        echo -e "  ${RED}●${RESET} ${BOLD}Erro:${RESET} Arquitetura não suportada: $ARCH"
        exit 1
        ;;
esac

# 2. Definir diretório de instalação
if [ -n "${BASH_SOURCE[0]}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
else
    SCRIPT_DIR="$PWD"
fi

if [ ! -f "$SCRIPT_DIR/start.sh" ]; then
    SCRIPT_DIR="$HOME/antigravity-server"
    mkdir -p "$SCRIPT_DIR"
fi

cd "$SCRIPT_DIR"
mkdir -p bin data uploads

OS_NAME="$OS"
if [ "$OS" = "darwin" ]; then
    OS_NAME="macOS"
elif [ "$OS" = "linux" ]; then
    OS_NAME="Linux"
fi
echo -e "  ${GREEN}●${RESET} ${BOLD}Sistema:${RESET}        ${OS_NAME} (${ARCH})"

# ------------------------------------------------------------------------------
# 3. Verificar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
AGY_PATH="$(which agy 2>/dev/null || true)"
if [ -z "$AGY_PATH" ] && [ -f "$HOME/.local/bin/agy" ]; then
    AGY_PATH="$HOME/.local/bin/agy"
    export PATH="$HOME/.local/bin:$PATH"
fi
if [ -z "$AGY_PATH" ] && [ -f "$HOME/.gemini/antigravity-cli/bin/agy" ]; then
    AGY_PATH="$HOME/.gemini/antigravity-cli/bin/agy"
    export PATH="$HOME/.gemini/antigravity-cli/bin:$PATH"
fi

if [ -n "$AGY_PATH" ]; then
    AGY_VER="$("$AGY_PATH" --version 2>/dev/null || echo 'instalado')"
    echo -e "  ${GREEN}●${RESET} ${BOLD}CLI Agente:${RESET}     Conectado (${AGY_PATH##*/} ${AGY_VER})"
else
    echo -e "  ${YELLOW}●${RESET} ${BOLD}CLI Agente:${RESET}     Instalando Google Antigravity CLI..."
    curl -fsSL https://antigravity.google/cli/install.sh | bash >/dev/null 2>&1 || true
    export PATH="$HOME/.local/bin:$HOME/.gemini/antigravity-cli/bin:$PATH"
    AGY_PATH="$(which agy 2>/dev/null || echo "$HOME/.local/bin/agy")"
    if [ -x "$AGY_PATH" ]; then
        echo -e "  ${GREEN}●${RESET} ${BOLD}CLI Agente:${RESET}     Instalado com sucesso"
    fi
fi

# ------------------------------------------------------------------------------
# 4. Verificar Autenticação Google no 'agy'
# ------------------------------------------------------------------------------
AUTH_OK=false
if [ -n "$AGY_PATH" ] && "$AGY_PATH" models >/dev/null 2>&1; then
    AUTH_OK=true
fi

if [ "$AUTH_OK" = true ]; then
    echo -e "  ${GREEN}●${RESET} ${BOLD}Autenticação:${RESET}   Conta Google vinculada"
else
    echo -e "  ${YELLOW}●${RESET} ${BOLD}Autenticação:${RESET}   Pendente (conecte via 'agy')"
fi

# ------------------------------------------------------------------------------
# 5. Verificar ou Baixar Cloudflare Tunnel (cloudflared)
# ------------------------------------------------------------------------------
CLOUDFLARED_BIN="$SCRIPT_DIR/bin/cloudflared"
SYSTEM_CLOUDFLARED="$(which cloudflared 2>/dev/null || true)"

if [ -x "$CLOUDFLARED_BIN" ] || [ -n "$SYSTEM_CLOUDFLARED" ]; then
    echo -e "  ${GREEN}●${RESET} ${BOLD}Túnel Seguro:${RESET}   Cloudflare Zero Trust pronto"
else
    DOWNLOAD_URL=""
    if [ "$OS" = "darwin" ]; then
        DOWNLOAD_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-${CLOUDFLARED_ARCH}.tgz"
        curl -fsSL "$DOWNLOAD_URL" -o /tmp/cloudflared.tgz
        tar -xzf /tmp/cloudflared.tgz -C "$SCRIPT_DIR/bin/"
        rm -f /tmp/cloudflared.tgz
    else
        DOWNLOAD_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CLOUDFLARED_ARCH}"
        curl -fsSL "$DOWNLOAD_URL" -o "$CLOUDFLARED_BIN"
    fi
    chmod +x "$CLOUDFLARED_BIN"
    echo -e "  ${GREEN}●${RESET} ${BOLD}Túnel Seguro:${RESET}   Cloudflare Zero Trust configurado"
fi

# ------------------------------------------------------------------------------
# 6. Baixar Interface Web se necessário
# ------------------------------------------------------------------------------
if [ -d "$SCRIPT_DIR/antigravity-server" ]; then
    if [ -d "$SCRIPT_DIR/antigravity-server/web" ] && [ ! -d "$SCRIPT_DIR/web" ]; then
        cp -R "$SCRIPT_DIR/antigravity-server/web" "$SCRIPT_DIR/"
    fi
    rm -rf "$SCRIPT_DIR/antigravity-server"
fi

if [ ! -f "$SCRIPT_DIR/web/index.html" ]; then
    ZIP_OS="$OS"
    if [ "$OS" = "darwin" ]; then
        ZIP_OS="macos"
    fi
    PKG_URL="https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-${ZIP_OS}.zip"
    curl -fsSL "$PKG_URL" -o /tmp/antigravity-server.zip 2>/dev/null || true
    if [ -f /tmp/antigravity-server.zip ]; then
        mkdir -p /tmp/antigravity_pkg_extract
        unzip -q -o /tmp/antigravity-server.zip -d /tmp/antigravity_pkg_extract 2>/dev/null || true
        if [ -d "/tmp/antigravity_pkg_extract/antigravity-server/web" ]; then
            cp -R /tmp/antigravity_pkg_extract/antigravity-server/web "$SCRIPT_DIR/"
        elif [ -d "/tmp/antigravity_pkg_extract/web" ]; then
            cp -R /tmp/antigravity_pkg_extract/web "$SCRIPT_DIR/"
        fi
        rm -rf /tmp/antigravity_pkg_extract /tmp/antigravity-server.zip
    fi
fi
echo -e "  ${GREEN}●${RESET} ${BOLD}Interface Web:${RESET}  Flutter Web client pronto"

# ------------------------------------------------------------------------------
# 7. Configurações (.env) e Scripts
# ------------------------------------------------------------------------------
curl -fsSL "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.sh?v=$(date +%s)" -o "$SCRIPT_DIR/start.sh"
chmod +x "$SCRIPT_DIR/start.sh"

if [ ! -f "$SCRIPT_DIR/.env" ]; then
    curl -fsSL "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/.env.example?v=$(date +%s)" -o "$SCRIPT_DIR/.env"
fi

# ------------------------------------------------------------------------------
# 8. Executável do Servidor
# ------------------------------------------------------------------------------
PREBUILT="$SCRIPT_DIR/bin/antigravity-server-${OS}-${GO_ARCH}"
if [ -f "$PREBUILT" ] && [ -f "$SCRIPT_DIR/main.go" ]; then
    cp "$PREBUILT" "$SCRIPT_DIR/antigravity-server"
else
    pkill -f "$SCRIPT_DIR/antigravity-server" 2>/dev/null || true
    EXE_URL="https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-${OS}-${GO_ARCH}?v=$(date +%s)"
    curl -fsSL "$EXE_URL" -o "$SCRIPT_DIR/antigravity-server.tmp"
    chmod +x "$SCRIPT_DIR/antigravity-server.tmp"
    mv -f "$SCRIPT_DIR/antigravity-server.tmp" "$SCRIPT_DIR/antigravity-server"
fi
chmod +x "$SCRIPT_DIR/antigravity-server"
echo -e "  ${GREEN}●${RESET} ${BOLD}Executável:${RESET}     Servidor nativo atualizado"

echo ""
echo -e "  ${GRAY}Iniciando servidor nativo...${RESET}"
echo ""

if [ -c /dev/tty ]; then
    exec "$SCRIPT_DIR/start.sh" < /dev/tty
else
    exec "$SCRIPT_DIR/start.sh"
fi
