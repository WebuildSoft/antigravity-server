#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Server - Instalador Inteligente (Linux & macOS)
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[0;32m"
YELLOW="\033[0;33m"
CYAN="\033[0;36m"
RED="\033[0;31m"
RESET="\033[0m"

echo -e "${BOLD}${CYAN}"
echo "============================================================"
echo "    🚀 INSTALADOR DO SERVIDOR ANTIGRAVITY MOBILE           "
echo "============================================================"
echo -e "${RESET}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

mkdir -p bin data uploads

# ------------------------------------------------------------------------------
# 1. Detectar Sistema Operacional e Arquitetura
# ------------------------------------------------------------------------------
echo -e "${BOLD}[1/5] Detectando Sistema e Arquitetura...${RESET}"
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
        echo -e "${RED}❌ Arquitetura não suportada: $ARCH${RESET}"
        exit 1
        ;;
esac

echo -e "      Sistema: ${GREEN}${OS}${RESET} | Arquitetura: ${GREEN}${ARCH}${RESET}"

# ------------------------------------------------------------------------------
# 2. Verificar ou Baixar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[2/5] Verificando Antigravity CLI ('agy')...${RESET}"

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
    AGY_VER="$("$AGY_PATH" --version 2>/dev/null || echo 'desconhecida')"
    echo -e "      ${GREEN}✓ agy encontrado:${RESET} $AGY_PATH (Versão: $AGY_VER)"
else
    echo -e "      ${YELLOW}! Antigravity CLI ('agy') não encontrado no sistema.${RESET}"
    echo -e "      ${CYAN}⬇ Baixando e instalando o agy automaticamente...${RESET}"
    if curl -fsSL https://antigravity.google/cli/install.sh | bash; then
        export PATH="$HOME/.local/bin:$PATH"
        AGY_PATH="$(which agy 2>/dev/null || echo "$HOME/.local/bin/agy")"
        echo -e "      ${GREEN}✓ agy instalado com sucesso!${RESET}"
    else
        echo -e "${RED}❌ Falha ao instalar o agy automaticamente.${RESET}"
        echo "Por favor instale manualmente com: curl -fsSL https://antigravity.google/cli/install.sh | bash"
        exit 1
    fi
fi

# ------------------------------------------------------------------------------
# 3. Verificar Autenticação Google no 'agy'
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[3/5] Verificando autenticação do 'agy'...${RESET}"

AUTH_OK=false
if "$AGY_PATH" models >/dev/null 2>&1; then
    AUTH_OK=true
fi

if [ "$AUTH_OK" = true ]; then
    echo -e "      ${GREEN}✓ agy autenticado e pronto para responder a comandos.${RESET}"
else
    echo -e "      ${YELLOW}! Nenhuma conta Google conectada ao agy.${RESET}"
    echo -e "      ${CYAN}🔑 Vamos iniciar a autenticação interativa agora...${RESET}"
    echo -e "      Uma janela ou link será exibido abaixo para você autorizar no navegador."
    echo ""
    "$AGY_PATH" || true
    echo ""
    if "$AGY_PATH" models >/dev/null 2>&1; then
        echo -e "      ${GREEN}✓ Login realizado com sucesso!${RESET}"
    else
        echo -e "      ${YELLOW}⚠️ O login não foi concluído. Você poderá autenticar depois executando 'agy' no terminal.${RESET}"
    fi
fi

# ------------------------------------------------------------------------------
# 4. Verificar ou Baixar Cloudflare Tunnel (cloudflared)
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[4/5] Verificando Cloudflare Tunnel (cloudflared)...${RESET}"

CLOUDFLARED_BIN="$SCRIPT_DIR/bin/cloudflared"
SYSTEM_CLOUDFLARED="$(which cloudflared 2>/dev/null || true)"

if [ -x "$CLOUDFLARED_BIN" ]; then
    echo -e "      ${GREEN}✓ cloudflared local encontrado em:${RESET} $CLOUDFLARED_BIN"
elif [ -n "$SYSTEM_CLOUDFLARED" ]; then
    echo -e "      ${GREEN}✓ cloudflared do sistema encontrado em:${RESET} $SYSTEM_CLOUDFLARED"
    ln -sf "$SYSTEM_CLOUDFLARED" "$CLOUDFLARED_BIN"
else
    echo -e "      ${YELLOW}! cloudflared não encontrado. Baixando binário oficial seguro...${RESET}"
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
    echo -e "      ${GREEN}✓ cloudflared baixado com sucesso em bin/cloudflared${RESET}"
fi

# ------------------------------------------------------------------------------
# 5. Configurar Arquivo .env e Permissões
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[5/5] Finalizando configurações...${RESET}"

if [ ! -f "$SCRIPT_DIR/.env" ] && [ -f "$SCRIPT_DIR/.env.example" ]; then
    cp "$SCRIPT_DIR/.env.example" "$SCRIPT_DIR/.env"
    echo -e "      ${GREEN}✓ Arquivo .env criado a partir de .env.example${RESET}"
fi

# Se o binário não existir na raiz, usar o pré-compilado ou compilar via Go
if [ ! -f "$SCRIPT_DIR/antigravity-server" ]; then
    PREBUILT="$SCRIPT_DIR/bin/antigravity-server-${OS}-${GO_ARCH}"
    if [ -f "$PREBUILT" ]; then
        echo -e "      ${GREEN}✓ Utilizando binário pré-compilado para ${OS}-${GO_ARCH}${RESET}"
        cp "$PREBUILT" "$SCRIPT_DIR/antigravity-server"
        chmod +x "$SCRIPT_DIR/antigravity-server"
    elif which go >/dev/null 2>&1; then
        echo -e "      ${CYAN}Compilando antigravity-server via Go...${RESET}"
        go build -ldflags="-s -w" -o "$SCRIPT_DIR/antigravity-server" .
    fi
fi

chmod +x "$SCRIPT_DIR/start.sh" 2>/dev/null || true
if [ -f "$SCRIPT_DIR/antigravity-server" ]; then
    chmod +x "$SCRIPT_DIR/antigravity-server"
fi

echo -e "\n${BOLD}${GREEN}============================================================"
echo "    🎉 INSTALAÇÃO CONCLUÍDA COM SUCESSO!                    "
echo -e "============================================================${RESET}"
echo -e "Para iniciar o servidor a qualquer momento, execute:"
echo -e "  ${BOLD}${CYAN}./start.sh${RESET}\n"
