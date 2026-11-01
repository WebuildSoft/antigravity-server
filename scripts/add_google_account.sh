#!/bin/bash
set -e

# ==============================================================================
# Helper para Adicionar Nova Conta Google no Antigravity CLI / Mobile
# ==============================================================================

GEMINI_DIR="$HOME/.gemini"
PROFILES_DIR="$GEMINI_DIR/profiles"
CREDS_FILE="$GEMINI_DIR/oauth_creds.json"
ACCOUNTS_FILE="$GEMINI_DIR/google_accounts.json"

mkdir -p "$PROFILES_DIR"

echo "=========================================================="
echo "  🔐 Antigravity - Assistente de Conexão de Nova Conta"
echo "=========================================================="

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
")
    if [ -n "$CURRENT_EMAIL" ]; then
        echo "💾 Salvando backup da conta atual ($CURRENT_EMAIL)..."
        mkdir -p "$PROFILES_DIR/$CURRENT_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$CURRENT_EMAIL/oauth_creds.json"
    fi
    mv "$CREDS_FILE" "$CREDS_FILE.bak"
fi

echo ""
echo "👉 Uma janela do navegador será aberta pelo Antigravity CLI."
echo "👉 Faça login com a NOVA CONTA GOOGLE que deseja adicionar."
echo "----------------------------------------------------------"

# Executa agy para disparar o login interativo do Google no navegador
agy || true

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
")
    if [ -n "$NEW_EMAIL" ]; then
        echo ""
        echo "🎉 Sucesso! Nova conta autenticada: $NEW_EMAIL"
        mkdir -p "$PROFILES_DIR/$NEW_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$NEW_EMAIL/oauth_creds.json"
        
        # Notifica o backend Go se estiver rodando
        curl -s -X POST http://localhost:4040/api/accounts/switch -H "Content-Type: application/json" -d "{\"email\": \"$NEW_EMAIL\"}" >/dev/null 2>&1 || true
        
        echo "✅ Conta $NEW_EMAIL registrada nos perfis!"
        echo "📱 Ela já está disponível no seu app mobile para alternar com 1 toque."
        rm -f "$CREDS_FILE.bak"
        exit 0
    fi
fi

# Se falhou ou cancelou, restaura o backup
if [ -f "$CREDS_FILE.bak" ]; then
    echo "⚠️ Login cancelado. Restaurando conta anterior..."
    mv "$CREDS_FILE.bak" "$CREDS_FILE"
fi

echo "❌ Nenhuma nova conta foi configurada."
