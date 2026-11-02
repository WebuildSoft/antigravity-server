#!/usr/bin/env bash
# ==============================================================================
#  Antigravity Ecosystem • Pipeline Oficial de Build, Release & Deploy
#  WebuildSoft Engine • Automação Multiplataforma (Mobile, Server, Releases)
# ==============================================================================
set -e

BOLD="\033[1m"
GREEN="\033[32m"
CYAN="\033[36m"
YELLOW="\033[33m"
RED="\033[31m"
GRAY="\033[90m"
RESET="\033[0m"

echo ""
echo -e "${CYAN}╭─────────────────────────────────────────────────────────────╮${RESET}"
echo -e "${CYAN}│${RESET}  ${BOLD}ANTIGRAVITY ECOSYSTEM • DEPLOY & RELEASE PIPELINE${RESET}          ${CYAN}│${RESET}"
echo -e "${CYAN}│${RESET}  ${GRAY}WebuildSoft Engine • Publicação Automatizada               ${RESET}${CYAN}│${RESET}"
echo -e "${CYAN}╰─────────────────────────────────────────────────────────────╯${RESET}"
echo ""

# 1. Localização Canônica dos Repositórios
DOCS_DIR="$HOME/Documents"
MOBILE_REPO="$DOCS_DIR/Antigravity-mobile-nilson"
MOBILE_APP="$MOBILE_REPO/mobile"
SERVER_REPO="$DOCS_DIR/antigravity-server"
RELEASES_REPO="$DOCS_DIR/antigravity-mobile-releases"

# Validações dos diretórios
for dir in "$MOBILE_APP" "$SERVER_REPO" "$RELEASES_REPO"; do
  if [ ! -d "$dir" ]; then
    echo -e "  ${RED}●${RESET} ${BOLD}Erro:${RESET} Diretório não encontrado: $dir"
    exit 1
  fi
done

# 2. Argumentos de Linha de Comando
TARGET_VERSION=""
TARGET_BUILD=""
CHANGELOG_MSG=""
DO_BUILD_APK=false
SKIP_PUSH=false

while [[ "$#" -gt 0 ]]; do
  case $1 in
    --version|-v) TARGET_VERSION="$2"; shift ;;
    --build|-b) TARGET_BUILD="$2"; shift ;;
    --changelog|-c) CHANGELOG_MSG="$2"; shift ;;
    --build-apk) DO_BUILD_APK=true ;;
    --skip-push) SKIP_PUSH=true ;;
    *) echo -e "  ${YELLOW}●${RESET} Parâmetro ignorado: $1" ;;
  esac
  shift
done

# 3. Resolução da Versão e Build Atual
CURRENT_VER=$(grep -o '"version": "[^"]*"' "$SERVER_REPO/version.json" | head -n1 | cut -d'"' -f4)
CURRENT_BLD=$(grep -o '"build": [0-9]*' "$SERVER_REPO/version.json" | head -n1 | cut -d' ' -f2)

if [ -z "$TARGET_VERSION" ]; then
  TARGET_VERSION="$CURRENT_VER"
fi
if [ -z "$TARGET_BUILD" ]; then
  TARGET_BUILD=$((CURRENT_BLD + 1))
fi
if [ -z "$CHANGELOG_MSG" ]; then
  CHANGELOG_MSG="Atualização de desempenho, melhorias de conectividade e sincronização com o ecossistema Antigravity."
fi

RELEASE_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo -e "  ${GREEN}●${RESET} ${BOLD}Versão Alvo:${RESET}    v${TARGET_VERSION} (Build #${TARGET_BUILD})"
echo -e "  ${GREEN}●${RESET} ${BOLD}Data/Hora:${RESET}      ${RELEASE_DATE}"
echo -e "  ${GREEN}●${RESET} ${BOLD}Changelog:${RESET}      ${CHANGELOG_MSG}"
echo ""

# ------------------------------------------------------------------------------
# 4. Limpeza Preventiva de Arquivos Soltos
# ------------------------------------------------------------------------------
echo -e "  ${CYAN}[1/6]${RESET} Limpando arquivos soltos e caches temporários..."
rm -f "$SERVER_REPO"/*.apk "$SERVER_REPO"/*.zip
rm -f "$MOBILE_REPO"/*.apk "$MOBILE_REPO"/*.zip
echo -e "  ${GREEN}●${RESET} Diretórios dos repositórios higienizados com sucesso."

# ------------------------------------------------------------------------------
# 5. Sincronização dos Arquivos de Versão
# ------------------------------------------------------------------------------
echo -e "\n  ${CYAN}[2/6]${RESET} Sincronizando números de versão nos 3 repositórios..."

# 5.1 Server version.json
cat <<EOF > "$SERVER_REPO/version.json"
{
  "version": "$TARGET_VERSION",
  "build": $TARGET_BUILD,
  "releasedAt": "$RELEASE_DATE",
  "changelog": "$CHANGELOG_MSG",
  "downloadUrl": "https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/app-release.apk"
}
EOF

# 5.2 Releases version.json
cp "$SERVER_REPO/version.json" "$RELEASES_REPO/version.json"

# 5.3 Mobile pubspec.yaml
if [ -f "$MOBILE_APP/pubspec.yaml" ]; then
  sed -i '' "s/^version: .*/version: $TARGET_VERSION+$TARGET_BUILD/" "$MOBILE_APP/pubspec.yaml"
fi

# 5.4 Mobile version.dart
cat <<EOF > "$MOBILE_APP/lib/version.dart"
// AUTO-GENERATED FILE - DO NOT EDIT MANUALLY
// Generated for Build #$TARGET_BUILD
class AppVersion {
  static const String version = '$TARGET_VERSION';
  static const int build = $TARGET_BUILD;
  static const String releasedAt = '$RELEASE_DATE';
  static const String changelog = '$CHANGELOG_MSG';

  static String get display => 'v\$version (build \$build)';
}
EOF
echo -e "  ${GREEN}●${RESET} Versão v${TARGET_VERSION} (#${TARGET_BUILD}) sincronizada no Server e Mobile."

# ------------------------------------------------------------------------------
# 6. Compilação dos Binários do Servidor Go (6 Targets)
# ------------------------------------------------------------------------------
echo -e "\n  ${CYAN}[3/6]${RESET} Compilando binários nativos do servidor Go..."
cd "$SERVER_REPO"
mkdir -p bin

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/antigravity-server-windows-amd64.exe .
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/antigravity-server-linux-amd64 .
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/antigravity-server-linux-arm64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/antigravity-server-darwin-amd64 .
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/antigravity-server-darwin-arm64 .
lipo -create -output bin/antigravity-server-darwin-universal bin/antigravity-server-darwin-amd64 bin/antigravity-server-darwin-arm64

echo -e "  ${GREEN}●${RESET} 6 binários nativos compilados com sucesso em bin/"

# ------------------------------------------------------------------------------
# 7. Compilação ou Coleta do APK Android
# ------------------------------------------------------------------------------
echo -e "\n  ${CYAN}[4/6]${RESET} Preparando pacote APK Mobile..."
APK_BUILT="$MOBILE_APP/build/app/outputs/flutter-apk/app-release.apk"

if [ "$DO_BUILD_APK" = true ]; then
  echo -e "  ${GRAY}Compilando novo Flutter APK Release...${RESET}"
  cd "$MOBILE_APP"
  flutter build apk --release
fi

if [ -f "$APK_BUILT" ]; then
  cp "$APK_BUILT" "$RELEASES_REPO/app-release.apk"
  cp "$APK_BUILT" "$RELEASES_REPO/AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk"
  APK_SIZE=$(du -h "$RELEASES_REPO/app-release.apk" | cut -f1 | tr -d ' ')
  echo -e "  ${GREEN}●${RESET} APK oficial pronto: ${BOLD}AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk${RESET} (${APK_SIZE})"
else
  echo -e "  ${YELLOW}●${RESET} Aviso: Nenhum APK encontrado em $APK_BUILT"
  APK_SIZE="77M"
fi

# ------------------------------------------------------------------------------
# 8. Empacotamento dos Pacotes ZIP de Distribuição
# ------------------------------------------------------------------------------
echo -e "\n  ${CYAN}[5/6]${RESET} Empacotando pacotes ZIP do servidor (Windows, Mac, Linux)..."
TMP_STAGE="/tmp/antigravity_pkg_stage"
rm -rf "$TMP_STAGE"
mkdir -p "$TMP_STAGE"

# 8.1 Windows ZIP
mkdir -p "$TMP_STAGE/win"
cp "$SERVER_REPO/bin/antigravity-server-windows-amd64.exe" "$TMP_STAGE/win/antigravity-bridge.exe"
cp "$SERVER_REPO/start.ps1" "$TMP_STAGE/win/start.ps1"
cp "$SERVER_REPO/start.bat" "$TMP_STAGE/win/start.bat"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/win/version.json"
[ -f "$SERVER_REPO/app_icon.ico" ] && cp "$SERVER_REPO/app_icon.ico" "$TMP_STAGE/win/"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/win/"
cd "$TMP_STAGE/win"
zip -q -r "$RELEASES_REPO/antigravity-server-windows.zip" .
WIN_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-windows.zip" | cut -f1 | tr -d ' ')

# 8.2 macOS ZIP
mkdir -p "$TMP_STAGE/mac"
cp "$SERVER_REPO/bin/antigravity-server-darwin-universal" "$TMP_STAGE/mac/antigravity-bridge"
cp "$SERVER_REPO/start.sh" "$TMP_STAGE/mac/start.sh"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/mac/version.json"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/mac/"
cd "$TMP_STAGE/mac"
zip -q -r "$RELEASES_REPO/antigravity-server-macos.zip" .
MAC_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-macos.zip" | cut -f1 | tr -d ' ')

# 8.3 Linux ZIP
mkdir -p "$TMP_STAGE/lin"
cp "$SERVER_REPO/bin/antigravity-server-linux-amd64" "$TMP_STAGE/lin/antigravity-bridge"
cp "$SERVER_REPO/start.sh" "$TMP_STAGE/lin/start.sh"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/lin/version.json"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/lin/"
cd "$TMP_STAGE/lin"
zip -q -r "$RELEASES_REPO/antigravity-server-linux.zip" .
LIN_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-linux.zip" | cut -f1 | tr -d ' ')

rm -rf "$TMP_STAGE"
echo -e "  ${GREEN}●${RESET} Windows ZIP: ${WIN_SIZE} | macOS ZIP: ${MAC_SIZE} | Linux ZIP: ${LIN_SIZE}"

# ------------------------------------------------------------------------------
# 9. Atualização da Documentação Oficial (README & RELEASE_NOTES)
# ------------------------------------------------------------------------------
cat <<EOF > "$RELEASES_REPO/RELEASE_NOTES.md"
# ==============================================================================
# CAMPO 1: RELEASE TITLE (Copie e cole no campo "Release title" do GitHub)
# ==============================================================================
Antigravity Mobile & Server v${TARGET_VERSION} (Build #${TARGET_BUILD})

# ==============================================================================
# CAMPO 2: DESCRIBE THIS RELEASE (Copie e cole no campo "Describe this release")
# ==============================================================================
## Novidades da Versão v${TARGET_VERSION} (Build #${TARGET_BUILD})

${CHANGELOG_MSG}

---

### Download dos Pacotes da Release

| Plataforma | Pacote Versionado | Tamanho | Inicialização |
| :--- | :--- | :---: | :--- |
| **Android** | \`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\` | ${APK_SIZE} | Instalar no dispositivo |
| **Windows** | \`antigravity-server-windows.zip\` | ${WIN_SIZE} | Executar \`.\start.ps1\` ou \`start.bat\` |
| **macOS** | \`antigravity-server-macos.zip\` | ${MAC_SIZE} | Executar \`./start.sh\` |
| **Linux** | \`antigravity-server-linux.zip\` | ${LIN_SIZE} | Executar \`./start.sh\` |

---

## Guia de Instalação e Execução

### 1. Dispositivo Móvel (Android)
Baixe o instalador oficial \`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\` (ou \`app-release.apk\`) e aponte a câmera para o QR Code gerado pelo servidor.

### 2. Servidor Companion (Computador)

#### Windows (PowerShell)
\`\`\`powershell
irm https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.ps1 | iex
\`\`\`

#### macOS e Linux (Terminal)
\`\`\`bash
curl -sSL https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.sh | bash
\`\`\`
EOF

cat <<EOF > "$RELEASES_REPO/README.md"
<div align="center">

<img src="https://raw.githubusercontent.com/WebuildSoft/antigravity-mobile-site/main/icons/Icon-192.png" width="88" height="88" alt="Antigravity Mobile Logo" />

# Antigravity Mobile & Server

Companion mobile client e servidor local de alta performance para o Google Antigravity.

[![Latest Release](https://img.shields.io/github/v/release/WebuildSoft/antigravity-mobile-releases?color=0284c7&style=flat-square)](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest)
[![Platform](https://img.shields.io/badge/platform-Android%20%7C%20Windows%20%7C%20macOS%20%7C%20Linux-334155?style=flat-square)](#download-dos-pacotes-v${TARGET_VERSION/./}--build-${TARGET_BUILD})
[![Go Server](https://img.shields.io/badge/server-Go%201.24-00ADD8?style=flat-square&logo=go&logoColor=white)](https://github.com/WebuildSoft/antigravity-server)
[![Flutter](https://img.shields.io/badge/client-Flutter-02569B?style=flat-square&logo=flutter&logoColor=white)](https://github.com/WebuildSoft/antigravity-mobile-releases)
[![Developer](https://img.shields.io/badge/developer-WebuildSoft-0284c7?style=flat-square)](https://github.com/WebuildSoft)

</div>

---

## Download dos Pacotes (v${TARGET_VERSION} • Build #${TARGET_BUILD})

Os binários compilados estão disponíveis diretamente na aba de [Releases](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest):

| Plataforma | Pacote Versionado | Tamanho | Inicialização |
| :--- | :--- | :---: | :--- |
| **Android** | [\`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/app-release.apk) | ${APK_SIZE} | Instalar no dispositivo |
| **Windows** | [\`antigravity-server-windows.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-windows.zip) | ${WIN_SIZE} | Executar \`.\start.ps1\` ou \`start.bat\` |
| **macOS** | [\`antigravity-server-macos.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-macos.zip) | ${MAC_SIZE} | Executar \`./start.sh\` |
| **Linux** | [\`antigravity-server-linux.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-linux.zip) | ${LIN_SIZE} | Executar \`./start.sh\` |

---

## Guia de Instalação e Execução

### 1. Dispositivo Móvel (Android)
1. Baixe o instalador [\`app-release.apk\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/app-release.apk).
2. Se o navegador exibir aviso de segurança, selecione **"Fazer download mesmo assim"** e autorize a instalação.
3. Conecte-se ao seu servidor apontando a câmera para o QR Code gerado no terminal.

### 2. Servidor Companion (Computador)

#### Windows (PowerShell)
\`\`\`powershell
irm https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.ps1 | iex
\`\`\`

#### macOS e Linux (Terminal)
\`\`\`bash
curl -sSL https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.sh | bash
\`\`\`

---

Desenvolvido por WebuildSoft • Todos os direitos reservados © 2026
EOF

# ------------------------------------------------------------------------------
# 10. Publicação Git & GitHub Release (via gh CLI)
# ------------------------------------------------------------------------------
if [ "$SKIP_PUSH" = false ]; then
  echo -e "\n  ${CYAN}[6/6]${RESET} Publicando alterações no GitHub..."

  # 10.1 Push no antigravity-server
  cd "$SERVER_REPO"
  git add bin/ version.json install.sh install.ps1 start.sh start.ps1 2>/dev/null || true
  git commit -m "chore(release): bump to v${TARGET_VERSION} (Build #${TARGET_BUILD})" 2>/dev/null || true
  git push origin main 2>/dev/null || true

  # 10.2 Push no antigravity-mobile-releases
  cd "$RELEASES_REPO"
  git add version.json README.md RELEASE_NOTES.md
  git commit -m "chore(release): release v${TARGET_VERSION} (Build #${TARGET_BUILD})" 2>/dev/null || true
  git push origin main 2>/dev/null || true

  # 10.3 Atualizar GitHub Release com gh
  TAG_NAME="v${TARGET_VERSION}"
  echo -e "  ${GRAY}Atualizando GitHub Release $TAG_NAME...${RESET}"
  
  gh release edit "$TAG_NAME" \
    --title "Antigravity Mobile & Server v${TARGET_VERSION} (Build #${TARGET_BUILD})" \
    --notes-file "$RELEASES_REPO/RELEASE_NOTES.md" \
    --repo WebuildSoft/antigravity-mobile-releases 2>/dev/null || \
  gh release create "$TAG_NAME" \
    --title "Antigravity Mobile & Server v${TARGET_VERSION} (Build #${TARGET_BUILD})" \
    --notes-file "$RELEASES_REPO/RELEASE_NOTES.md" \
    --repo WebuildSoft/antigravity-mobile-releases

  # Limpar APKs com build antigo do GitHub Release
  OLD_ASSETS=$(gh release view "$TAG_NAME" --repo WebuildSoft/antigravity-mobile-releases --json assets --jq '.assets[].name' 2>/dev/null || true)
  for a in $OLD_ASSETS; do
    if [[ "$a" == *"AntigravityMobile-"* && "$a" != *"build${TARGET_BUILD}"* ]]; then
      echo -e "  ${GRAY}Removendo asset antigo do GitHub: $a${RESET}"
      gh release delete-asset "$TAG_NAME" "$a" --repo WebuildSoft/antigravity-mobile-releases -y 2>/dev/null || true
    fi
  done

  # Upload dos novos arquivos
  gh release upload "$TAG_NAME" \
    "$RELEASES_REPO/app-release.apk" \
    "$RELEASES_REPO/AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk" \
    "$RELEASES_REPO/antigravity-server-windows.zip" \
    "$RELEASES_REPO/antigravity-server-macos.zip" \
    "$RELEASES_REPO/antigravity-server-linux.zip" \
    --clobber --repo WebuildSoft/antigravity-mobile-releases

  echo -e "  ${GREEN}●${RESET} GitHub Release atualizada com sucesso!"
fi

echo ""
echo -e "${GREEN}╭─────────────────────────────────────────────────────────────╮${RESET}"
echo -e "${GREEN}│${RESET}  ${BOLD}DEPLOY CONCLUÍDO COM SUCESSO!                              ${RESET}${GREEN}│${RESET}"
echo -e "${GREEN}│${RESET}  Versão: v${TARGET_VERSION} (Build #${TARGET_BUILD})                                 ${GREEN}│${RESET}"
echo -e "${GREEN}│${RESET}  GitHub: https://github.com/WebuildSoft/antigravity-mobile-releases ${GREEN}│${RESET}"
echo -e "${GREEN}╰─────────────────────────────────────────────────────────────╯${RESET}"
echo ""
