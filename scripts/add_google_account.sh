#!/usr/bin/env bash
set -e

# ==============================================================================
# Helper para Adicionar Nova Conta Google no Antigravity CLI / Mobile
# ==============================================================================

GEMINI_DIR="$HOME/.gemini"
PROFILES_DIR="$GEMINI_DIR/profiles"
CREDS_FILE="$GEMINI_DIR/oauth_creds.json"
ACCOUNTS_FILE="$GEMINI_DIR/google_accounts.json"
PORT="${PORT:-4040}"

mkdir -p "$PROFILES_DIR"

echo ""
echo "=========================================================="
echo "  🔐 Antigravity - Assistente de Conexão de Nova Conta"
echo "=========================================================="
echo ""

# Backup conta atual se existir
if [ -f "$CREDS_FILE" ]; then
    CURRENT_EMAIL=$(python3 -c "
import json, base64, sys
try:
    with open('$CREDS_FILE') as f: d = json.load(f)
    t = d.get('id_token', '')
    if t:
        p = t.split('.')[1] + '=='
        print(json.loads(base64.urlsafe_b64decode(p[:len(p)-len(p)%4])).get('email', ''))
except: pass
" 2>/dev/null || true)
    if [ -n "$CURRENT_EMAIL" ]; then
        echo "💾 Salvando backup da conta atual ($CURRENT_EMAIL)..."
        mkdir -p "$PROFILES_DIR/$CURRENT_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$CURRENT_EMAIL/oauth_creds.json"
    fi
    cp "$CREDS_FILE" "$CREDS_FILE.bak"
    mv "$CREDS_FILE" "$CREDS_FILE.temp_switch"
fi

restore_backup() {
    if [ ! -s "$CREDS_FILE" ]; then
        if [ -f "$CREDS_FILE.temp_switch" ]; then
            mv "$CREDS_FILE.temp_switch" "$CREDS_FILE" 2>/dev/null || true
        elif [ -f "$CREDS_FILE.bak" ]; then
            cp "$CREDS_FILE.bak" "$CREDS_FILE" 2>/dev/null || true
        fi
    else
        rm -f "$CREDS_FILE.temp_switch" 2>/dev/null || true
    fi
}
trap restore_backup EXIT INT TERM

echo "👉 Uma janela do navegador será aberta pelo Antigravity CLI oficial."
echo "👉 Faça login com a NOVA CONTA GOOGLE que deseja adicionar."
echo "👉 Depois de autorizar no navegador, digite 'exit' no terminal para concluir."
echo "----------------------------------------------------------"

AGY_BIN="$(which agy 2>/dev/null || echo "$HOME/.local/bin/agy")"
if [ ! -x "$AGY_BIN" ]; then
    AGY_BIN="$HOME/.gemini/antigravity-cli/bin/agy"
fi

"$AGY_BIN" || true

if [ -f "$CREDS_FILE" ]; then
    NEW_EMAIL=$(python3 -c "
import json, base64
try:
    with open('$CREDS_FILE') as f: d = json.load(f)
    t = d.get('id_token', '')
    if t:
        p = t.split('.')[1] + '=='
        print(json.loads(base64.urlsafe_b64decode(p[:len(p)-len(p)%4])).get('email', ''))
except: pass
" 2>/dev/null || true)
    if [ -n "$NEW_EMAIL" ]; then
        echo ""
        echo "🎉 Sucesso! Nova conta autenticada: $NEW_EMAIL"
        mkdir -p "$PROFILES_DIR/$NEW_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$NEW_EMAIL/oauth_creds.json"
        
        # Notifica o backend Go
        curl -s -X POST "http://localhost:${PORT}/api/accounts/switch" -H "Content-Type: application/json" -d "{\"email\": \"$NEW_EMAIL\"}" >/dev/null 2>&1 || true
        
        echo "✅ Conta $NEW_EMAIL registrada nos perfis!"
        echo "📱 Ela já está disponível no seu app mobile para alternar com 1 toque."
        rm -f "$CREDS_FILE.bak"
        echo ""
        read -p "Pressione Enter para fechar esta janela..."
        exit 0
    fi
fi

# Se falhou ou cancelou, restaura o backup
if [ -f "$CREDS_FILE.bak" ]; then
    echo "⚠️ Login cancelado ou não concluído. Restaurando conta anterior..."
    mv "$CREDS_FILE.bak" "$CREDS_FILE"
fi

echo "❌ Nenhuma nova conta foi configurada."
echo ""
read -p "Pressione Enter para fechar esta janela..."
