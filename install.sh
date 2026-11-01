#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Mobile Server - Instalador Inteligente Multiplataforma
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[32m"
YELLOW="\033[33m"
CYAN="\033[36m"
RED="\033[31m"
RESET="\033[0m"

echo -e "${BOLD}${CYAN}"
echo "============================================================"
echo "    🚀 INSTALADOR DO SERVIDOR ANTIGRAVITY MOBILE           "
echo "============================================================"
echo -e "${RESET}"

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
        echo -e "${RED}❌ Arquitetura não suportada: $ARCH${RESET}"
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

echo -e "${BOLD}[1/4] Sistema:${RESET} ${GREEN}${OS}${RESET} (${GREEN}${ARCH}${RESET})"

# ------------------------------------------------------------------------------
# 3. Verificar ou Baixar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[2/4] Verificando Antigravity CLI ('agy')...${RESET}"

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
# Verificar Autenticação Google no 'agy'
# ------------------------------------------------------------------------------
echo -e "\nVerificando autenticação Google no Antigravity..."

AUTH_OK=false
if "$AGY_PATH" models >/dev/null 2>&1; then
    AUTH_OK=true
fi

if [ "$AUTH_OK" = true ]; then
    echo -e "      ${GREEN}✓ agy autenticado e pronto para responder a comandos.${RESET}"
else
    echo -e "      ${YELLOW}! Nenhuma conta Google conectada ao agy.${RESET}"
    if [ -t 0 ]; then
        echo -e "      ${CYAN}🔑 Vamos iniciar a autenticação interativa agora...${RESET}"
        echo -e "      Uma janela ou link será exibido abaixo para você autorizar no navegador."
        echo ""
        "$AGY_PATH" || true
        echo ""
        if "$AGY_PATH" models >/dev/null 2>&1; then
            echo -e "      ${GREEN}✓ Login realizado com sucesso!${RESET}"
        else
            echo -e "      ${YELLOW}⚠️ O login não foi concluído. O servidor ainda iniciará normalmente.${RESET}"
        fi
    elif [ -c /dev/tty ]; then
        echo -e "      ${CYAN}🔑 Vamos iniciar a autenticação interativa agora...${RESET}"
        echo -e "      Uma janela ou link será exibido abaixo para você autorizar no navegador."
        echo ""
        "$AGY_PATH" < /dev/tty || true
        echo ""
        if "$AGY_PATH" models >/dev/null 2>&1; then
            echo -e "      ${GREEN}✓ Login realizado com sucesso!${RESET}"
        fi
    else
        echo -e "      ${YELLOW}⚠️ Execute 'agy' no terminal quando desejar conectar sua conta Google.${RESET}"
    fi
fi

# ------------------------------------------------------------------------------
# 4. Verificar ou Baixar Cloudflare Tunnel (cloudflared)
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[3/4] Verificando Cloudflare Tunnel (cloudflared)...${RESET}"

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
        curl -# -fSL "$DOWNLOAD_URL" -o /tmp/cloudflared.tgz
        tar -xzf /tmp/cloudflared.tgz -C "$SCRIPT_DIR/bin/"
        rm -f /tmp/cloudflared.tgz
    else
        DOWNLOAD_URL="https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-${CLOUDFLARED_ARCH}"
        curl -# -fSL "$DOWNLOAD_URL" -o "$CLOUDFLARED_BIN"
    fi
    chmod +x "$CLOUDFLARED_BIN"
    echo -e "      ${GREEN}✓ cloudflared baixado com sucesso em bin/cloudflared${RESET}"
fi

# ------------------------------------------------------------------------------
# 5. Baixar / Atualizar start.sh, .env e Executável
# ------------------------------------------------------------------------------
echo -e "\n${BOLD}[4/4] Configurando arquivos e executável...${RESET}"

curl -fsSL "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.sh" -o "$SCRIPT_DIR/start.sh"
chmod +x "$SCRIPT_DIR/start.sh"

if [ ! -f "$SCRIPT_DIR/.env" ]; then
    curl -fsSL "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/.env.example" -o "$SCRIPT_DIR/.env"
    echo -e "      ${GREEN}✓ Arquivo .env configurado com sucesso${RESET}"
fi

if [ ! -f "$SCRIPT_DIR/antigravity-server" ]; then
    PREBUILT="$SCRIPT_DIR/bin/antigravity-server-${OS}-${GO_ARCH}"
    if [ -f "$PREBUILT" ]; then
        echo -e "      ${GREEN}✓ Utilizando binário pré-compilado para ${OS}-${GO_ARCH}${RESET}"
        cp "$PREBUILT" "$SCRIPT_DIR/antigravity-server"
    else
        echo -e "      ${CYAN}⬇ Baixando executável do servidor para ${OS}-${GO_ARCH}...${RESET}"
        EXE_URL="https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-${OS}-${GO_ARCH}"
        curl -# -fSL "$EXE_URL" -o "$SCRIPT_DIR/antigravity-server"
        echo -e "      ${GREEN}✓ Executável antigravity-server pronto${RESET}"
    fi
fi
chmod +x "$SCRIPT_DIR/antigravity-server"

echo -e "\n${BOLD}${GREEN}============================================================"
echo "    🎉 INSTALAÇÃO CONCLUÍDA COM SUCESSO!                    "
echo -e "============================================================${RESET}"
echo -e "Para iniciar o servidor no futuro a qualquer momento, execute:"
echo -e "  ${CYAN}cd \"$SCRIPT_DIR\" && ./start.sh${RESET}\n"
echo -e "${BOLD}${GREEN}🚀 Iniciando o servidor automaticamente agora...${RESET}\n"

exec "$SCRIPT_DIR/start.sh"
