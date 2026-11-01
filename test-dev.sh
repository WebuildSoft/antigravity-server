#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Server - Suíte de Testes de Segurança em Desenvolvimento
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[0;32m"
CYAN="\033[0;36m"
RED="\033[0;31m"
RESET="\033[0m"

echo -e "${BOLD}${CYAN}"
echo "============================================================"
echo "    🛡️  TESTES DE SEGURANÇA E ISOLAMENTO (DEV MODE)        "
echo "============================================================"
echo -e "${RESET}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if ! which go >/dev/null 2>&1; then
    echo -e "${RED}❌ O Go é necessário para rodar os testes de desenvolvimento.${RESET}"
    exit 1
fi

echo -e "Executando testes automatizados com Go:\n"

if go test -v -count=1 ./tests; then
    echo -e "\n${BOLD}${GREEN}============================================================"
    echo "    ✅ TODOS OS TESTES PASSARAM COM SUCESSO!               "
    echo -e "============================================================${RESET}"
    echo -e "Camada 1 (Permissão Localhost):            ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 2 (Bloqueio sem Token):             ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 2 (Bloqueio Token Forjado):         ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 3 (Limite Upload 25MB & Anti-DoS):  ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 4 (Isolamento entre Usuários):      ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 5 (Hardening HTTP & Anti-Slowloris):${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 6 (Proteção Workspace Anti-LFI):    ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 7 (Persistência SQLite & ToolCalls):${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 8 (Resiliência & Fallback Portas):  ${GREEN}[PASSOU]${RESET}"
    echo -e "Camada 9 (Telemetria Discord & Anti-Crash):${GREEN}[PASSOU]${RESET}\n"
else
    echo -e "\n${BOLD}${RED}❌ ALGUNS TESTES FALHARAM. VERIFIQUE OS LOGS ACIMA.${RESET}\n"
    exit 1
fi
