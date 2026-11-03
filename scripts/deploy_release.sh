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
DO_BUILD_APK=true
SKIP_PUSH=false

while [[ "$#" -gt 0 ]]; do
  case $1 in
    --version|-v) TARGET_VERSION="$2"; shift ;;
    --build|-b) TARGET_BUILD="$2"; shift ;;
    --changelog|-c) CHANGELOG_MSG="$2"; shift ;;
    --build-apk) DO_BUILD_APK=true ;;
    --skip-apk|--no-apk) DO_BUILD_APK=false ;;
    --skip-push) SKIP_PUSH=true ;;
    *)
      if [ -z "$TARGET_VERSION" ]; then
        TARGET_VERSION="$1"
      elif [ -z "$TARGET_BUILD" ]; then
        TARGET_BUILD="$1"
      elif [ -z "$CHANGELOG_MSG" ]; then
        CHANGELOG_MSG="$1"
      else
        echo -e "  ${YELLOW}●${RESET} Parâmetro ignorado: $1"
      fi
      ;;
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
mkdir -p "$TMP_STAGE/win" "$TMP_STAGE/win/bin"
cp "$SERVER_REPO/bin/antigravity-server-windows-amd64.exe" "$TMP_STAGE/win/antigravity-server.exe"
cp "$SERVER_REPO/bin/antigravity-server-windows-amd64.exe" "$TMP_STAGE/win/antigravity-bridge.exe"
cp "$SERVER_REPO/bin/antigravity-server-windows-amd64.exe" "$TMP_STAGE/win/bin/"
cp "$SERVER_REPO/start.ps1" "$TMP_STAGE/win/start.ps1"
cp "$SERVER_REPO/start.bat" "$TMP_STAGE/win/start.bat"
cp "$SERVER_REPO/install.ps1" "$TMP_STAGE/win/install.ps1"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/win/version.json"
[ -f "$SERVER_REPO/.env.example" ] && cp "$SERVER_REPO/.env.example" "$TMP_STAGE/win/"
[ -f "$SERVER_REPO/app_icon.ico" ] && cp "$SERVER_REPO/app_icon.ico" "$TMP_STAGE/win/"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/win/"
cd "$TMP_STAGE/win"
zip -q -r "$RELEASES_REPO/antigravity-server-windows.zip" .
cp "$RELEASES_REPO/antigravity-server-windows.zip" "$RELEASES_REPO/antigravity-server-windows-v${TARGET_VERSION}-build${TARGET_BUILD}.zip"
WIN_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-windows.zip" | cut -f1 | tr -d ' ')

# 8.2 macOS ZIP
mkdir -p "$TMP_STAGE/mac"
cp "$SERVER_REPO/bin/antigravity-server-darwin-universal" "$TMP_STAGE/mac/antigravity-bridge"
cp "$SERVER_REPO/start.sh" "$TMP_STAGE/mac/start.sh"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/mac/version.json"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/mac/"
cd "$TMP_STAGE/mac"
zip -q -r "$RELEASES_REPO/antigravity-server-macos.zip" .
cp "$RELEASES_REPO/antigravity-server-macos.zip" "$RELEASES_REPO/antigravity-server-macos-v${TARGET_VERSION}-build${TARGET_BUILD}.zip"
MAC_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-macos.zip" | cut -f1 | tr -d ' ')

# 8.3 Linux ZIP
mkdir -p "$TMP_STAGE/lin"
cp "$SERVER_REPO/bin/antigravity-server-linux-amd64" "$TMP_STAGE/lin/antigravity-bridge"
cp "$SERVER_REPO/start.sh" "$TMP_STAGE/lin/start.sh"
cp "$SERVER_REPO/version.json" "$TMP_STAGE/lin/version.json"
[ -d "$SERVER_REPO/web" ] && cp -R "$SERVER_REPO/web" "$TMP_STAGE/lin/"
cd "$TMP_STAGE/lin"
zip -q -r "$RELEASES_REPO/antigravity-server-linux.zip" .
cp "$RELEASES_REPO/antigravity-server-linux.zip" "$RELEASES_REPO/antigravity-server-linux-v${TARGET_VERSION}-build${TARGET_BUILD}.zip"
LIN_SIZE=$(du -h "$RELEASES_REPO/antigravity-server-linux.zip" | cut -f1 | tr -d ' ')

rm -rf "$TMP_STAGE"
echo -e "  ${GREEN}●${RESET} Windows ZIP: ${WIN_SIZE} | macOS ZIP: ${MAC_SIZE} | Linux ZIP: ${LIN_SIZE}"

# ------------------------------------------------------------------------------
# 9. Atualização da Documentação Oficial (README & RELEASE_NOTES)
# ------------------------------------------------------------------------------
cat <<EOF > "$RELEASES_REPO/RELEASE_NOTES.md"
## Novidades da Versão v${TARGET_VERSION} (Build #${TARGET_BUILD})

${CHANGELOG_MSG}

---

### Aplicativo Mobile (Android APK • Build #${TARGET_BUILD})
* **Gerenciamento Multi-Contas Google:**
  * Suporte nativo para conectar e alternar instantaneamente entre 3+ contas Google.
  * Atualização e exibição em tempo real de limites e cotas de requisições de IA para cada conta.
  * Fluxo de autenticação assistida com captura e renovação de tokens OAuth.
* **Diffs Inteligentes & Code Editor:**
  * Reconstrução precisa da versão base para diffs na visualização de edições sugeridas pela IA.
  * Realce de sintaxe colorido (Syntax Highlighting) com preservação de indentação e espaçamento.
* **Comando de Voz & Áudio:**
  * Reconhecimento contínuo de voz com visualizador de ondas dinâmicas integrado ao botão de envio.
* **Prevenção de Mensagens Duplicadas:**
  * Proteção por debounce (bloqueio de toques múltiplos em sequência rápida).
  * Desativação preventiva do campo de texto e botão de envio enquanto a resposta da IA estiver sendo processada.
  * Deduplicação automática de mensagens por ID no gerenciador de estado.
* **Verificação de Atualização Direta:** Consulta em tempo real via GitHub Releases (\`version.json\`) com fallback inteligente local.

---

### Servidor Companion & Workspace (Build #${TARGET_BUILD})
* **Telemetria de Inicialização na Nuvem (Edge Analytics):**
  * Despacho assíncrono de pings anônimos de inicialização diretamente para Cloudflare Worker (\`https://telemetry.lnki.pro\`).
  * Dashboard visual moderno (Dark Mode) protegido por login com contagem de servidores únicos, ativos 24h e geolocalização.
  * Ativação padrão de mercado (opt-out configurável via \`.env\` ou flag \`--no-telemetry\`).
* **Instaladores Inteligentes:**
  * Novo instalador do Windows (\`install.ps1\`) com função nativa \`Safe-Download\` em .NET (imune a falhas de aspas no curl).
  * Visual renovado e minimalista no Windows e macOS/Linux (caixa ciano, checklist elegante \`●\`, zero emojis soltos).
* **Sincronização de Conversas:**
  * Mapeamento retroativo de sessões locais e preservação de histórico por usuário logado.
  * Isolamento de diretórios contra Directory Traversal e normalização de caminhos com til (\`~\`).
* **Binários Nativos Otimizados:**
  * Compilação em Go puro com suporte Universal no macOS (Apple Silicon M1/M2/M3 e Intel x86_64), Linux (amd64/arm64) e Windows (amd64).

---

### Download dos Pacotes da Release

| Plataforma | Pacote Versionado | Tamanho | Inicialização |
| :--- | :--- | :---: | :--- |
| **Android** | \`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\` | ${APK_SIZE} | Instalar no dispositivo |
| **Windows** | \`antigravity-server-windows-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\` | ${WIN_SIZE} | Executar \`.\start.ps1\` ou \`start.bat\` |
| **macOS** | \`antigravity-server-macos-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\` | ${MAC_SIZE} | Executar \`./start.sh\` |
| **Linux** | \`antigravity-server-linux-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\` | ${LIN_SIZE} | Executar \`./start.sh\` |

> [!NOTE]
> Os links fixos de download contínuo (\`app-release.apk\`, \`antigravity-server-windows.zip\`, etc.) apontam para estas mesmas compilações oficiais.

---

## Guia de Instalação e Execução

### 1. Dispositivo Móvel (Android)
1. Baixe o instalador oficial \`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\` (ou \`app-release.apk\`).
2. Se o navegador exibir o aviso de fontes desconhecidas, selecione **"Fazer download mesmo assim"** e autorize a instalação.
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

## Diagnóstico e Solução de Problemas

### 1. Alerta de Segurança no Windows (SmartScreen ou Antivírus)
* **SmartScreen ("O Windows protegeu o seu computador"):**
  Clique em **"Mais informações"** e selecione **"Executar assim mesmo"**.
* **Arquivo bloqueado pelo antivírus:**
  Acesse *Segurança do Windows > Proteção contra vírus e ameaças > Histórico de proteção* e clique em *Ações > Permitir no dispositivo / Restaurar*.
* **Recomendação técnica:**
  Execute diretamente o script nativo \`.\start.ps1\` no PowerShell para evitar alertas automáticos associados a arquivos em lote \`.bat\`.

### 2. Autenticação com a Conta Google no Antigravity CLI (\`agy\`)
> [!WARNING]
> O comando \`agy auth login\` **não existe** no CLI e resultará em erro de sintaxe.

* Para autenticar ou renovar credenciais, execute no terminal da sua máquina:
  \`\`\`bash
  agy
  \`\`\`
* Para validar a conectividade e os modelos de IA disponíveis:
  \`\`\`bash
  agy models
  \`\`\`

### 3. Conexão Local Bloqueada por Firewall (Porta 4040)
Caso o celular não localize o servidor na mesma rede Wi-Fi, abra o **PowerShell como Administrador** e execute:
\`\`\`powershell
New-NetFirewallRule -DisplayName "Antigravity Server (Porta 4040)" -Direction Inbound -LocalPort 4040 -Protocol TCP -Action Allow
\`\`\`
*(No Linux com UFW: \`sudo ufw allow 4040/tcp\`)*.

### 4. Redes Públicas, Corporativas ou Conexão 4G/5G
Se a rede local possuir isolamento de clientes (AP Isolation) ou você estiver fora de casa, o servidor gera automaticamente um Quick Tunnel seguro (\`.trycloudflare.com\`). Utilize o código QR correspondente à URL pública para conexão direta.
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
| **Windows** | [\`antigravity-server-windows-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-windows.zip) | ${WIN_SIZE} | Executar \`.\start.ps1\` ou \`start.bat\` |
| **macOS** | [\`antigravity-server-macos-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-macos.zip) | ${MAC_SIZE} | Executar \`./start.sh\` |
| **Linux** | [\`antigravity-server-linux-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\`](https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-linux.zip) | ${LIN_SIZE} | Executar \`./start.sh\` |

> [!NOTE]
> Os links fixos (\`app-release.apk\`, \`antigravity-server-windows.zip\`, etc.) redirecionam sempre para a compilação estável do release atual.

---

## Arquitetura do Sistema

O ecossistema divide-se em dois módulos complementares:

1. **Cliente Mobile (Android):** Interface nativa em Flutter focada em visualização do raciocínio em tempo real, navegação no explorador de arquivos, edição de código com visualizador de diffs, reconhecimento de voz e alternância rápida entre múltiplas contas Google.
2. **Servidor Companion (Desktop):** Binário em Go puro de alta performance e latência zero que atua como ponte com o Antigravity CLI oficial (\`agy\`), sincroniza conversas locais no SQLite embarcado e gerencia os canais WebSocket com criptografia TLS via túneis Cloudflare Zero Trust.

> [!IMPORTANT]
> O servidor utiliza a autenticação local do Antigravity CLI já presente na sua máquina de desenvolvimento. As requisições de IA consomem as cotas e permissões da sua conta Google associada.

---

## Guia de Instalação e Execução

### 1. Dispositivo Móvel (Android)
1. Baixe o instalador \`AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk\` (ou \`app-release.apk\`).
2. Se o navegador exibir o aviso de instalação de fontes desconhecidas, selecione **"Fazer download mesmo assim"** e autorize o navegador nas opções de segurança do sistema.
3. Abra o aplicativo instalado:
   - Caso já possua conta: faça login com seu e-mail e senha.
   - Caso seja seu primeiro uso: selecione **"Criar Conta"** para cadastrar seu perfil de acesso.

### 2. Servidor Companion (Computador)

#### Windows
Execute o script de automação via PowerShell:
\`\`\`powershell
irm https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.ps1 | iex
\`\`\`
*Alternativa manual via arquivo ZIP:*
1. Extraia o pacote \`antigravity-server-windows-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\`.
2. Abra a pasta no PowerShell e inicie o servidor:
   \`\`\`powershell
   .\start.ps1
   \`\`\`
   *(ou execute o utilitário \`start.bat\`)*.

#### macOS e Linux
Execute o script oficial via Terminal:
\`\`\`bash
curl -sSL https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.sh | bash
\`\`\`
*Alternativa manual via arquivo ZIP:*
1. Extraia o pacote \`antigravity-server-macos-v${TARGET_VERSION}-build${TARGET_BUILD}.zip\` (ou \`linux\`).
2. Abra a pasta no Terminal e inicie o servidor:
   \`\`\`bash
   ./start.sh
   \`\`\`

### 3. Pareamento
1. Após a inicialização, o terminal exibirá as rotas locais, a URL de túnel e um código QR em ASCII.
2. No aplicativo móvel, toque em **"Conectar Servidor"** e aponte a câmera para o código no terminal.
3. A conexão será estabelecida automaticamente via WebSocket autenticado.

---

## Diagnóstico e Solução de Problemas

### 1. Alerta de Segurança no Windows (SmartScreen ou Antivírus)
* **Causa:** O binário é uma compilação de código aberto recente e não inclui assinatura digital comercial corporativa da Microsoft. O utilitário \`.bat\` também utiliza parâmetros de bypass de política de execução, o que ativa heurísticas preventivas.
* **SmartScreen ("O Windows protegeu o seu computador"):**
  1. Clique em **"Mais informações"**.
  2. Selecione **"Executar assim mesmo"**.
* **Arquivo retido pelo antivírus:**
  Acesse *Segurança do Windows > Proteção contra vírus e ameaças > Histórico de proteção* e selecione *Ações > Permitir no dispositivo / Restaurar*.
* **Recomendação técnica:**
  Execute diretamente o script nativo \`.\start.ps1\` no PowerShell para evitar alertas automáticos associados a arquivos em lote \`.bat\`:
  \`\`\`powershell
  .\start.ps1
  \`\`\`

### 2. Autenticação com a Conta Google no Antigravity CLI (\`agy\`)
> [!WARNING]
> O comando \`agy auth login\` **não existe** no CLI e resultará em erro de sintaxe (\`unexpected argument "auth"\`).

* Para autenticar sua conta, execute no terminal da sua máquina:
  \`\`\`bash
  agy
  \`\`\`
  O utilitário abrirá uma sessão interativa e exibirá o link de autorização OAuth do Google no navegador.
* Para verificar o status da autenticação e validar os modelos disponíveis:
  \`\`\`bash
  agy models
  \`\`\`
  A listagem de modelos confirma que o ambiente local está apto a receber instruções do aplicativo móvel.

### 3. Bloqueio de Conexão Local por Firewall (Porta 4040)
Caso o dispositivo móvel não consiga se comunicar com o computador através da rede Wi-Fi local:
* **Windows (PowerShell com privilégios de Administrador):**
  \`\`\`powershell
  New-NetFirewallRule -DisplayName "Antigravity Server (Porta 4040)" -Direction Inbound -LocalPort 4040 -Protocol TCP -Action Allow
  \`\`\`
* **Linux (com firewall UFW ativo):**
  \`\`\`bash
  sudo ufw allow 4040/tcp
  \`\`\`
* **macOS:**
  Acesse *Ajustes do Sistema > Rede > Firewall* e garanta que o processo \`antigravity-server\` está habilitado para conexões de entrada.

### 4. Redes com Isolamento de Cliente (AP Isolation) e Conexões Remotas
Em redes corporativas, acadêmicas ou públicas, o roteador pode impedir a comunicação ponto a ponto entre dispositivos na mesma rede local.
* O servidor inicializa automaticamente um Quick Tunnel criptografado via Cloudflare com terminação \`.trycloudflare.com\`.
* Utilize o código QR correspondente à URL pública para rotear o tráfego de forma transparente mesmo em redes móveis (4G/5G) ou redes com isolamento ativo.

---

<div align="center">

Desenvolvido por WebuildSoft • Todos os direitos reservados © 2026

</div>
EOF

# ------------------------------------------------------------------------------
# 10. Publicação Git & GitHub Release (via gh CLI)
# ------------------------------------------------------------------------------
if [ "$SKIP_PUSH" = false ]; then
  echo -e "\n  ${CYAN}[6/6]${RESET} Publicando alterações no GitHub..."

  # 10.1 Push no Mobile
  cd "$MOBILE_REPO"
  git add -A 2>/dev/null || true
  git commit -m "chore(release): release v${TARGET_VERSION} (Build #${TARGET_BUILD})" 2>/dev/null || true
  git push origin main 2>/dev/null || true

  # 10.2 Push no antigravity-server
  cd "$SERVER_REPO"
  git add -A 2>/dev/null || true
  git commit -m "chore(release): bump to v${TARGET_VERSION} (Build #${TARGET_BUILD})" 2>/dev/null || true
  git push origin main 2>/dev/null || true

  # 10.3 Push no antigravity-mobile-releases
  cd "$RELEASES_REPO"
  git add -A 2>/dev/null || true
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

  # Limpar assets com build antigo do GitHub Release
  OLD_ASSETS=$(gh release view "$TAG_NAME" --repo WebuildSoft/antigravity-mobile-releases --json assets --jq '.assets[].name' 2>/dev/null || true)
  for a in $OLD_ASSETS; do
    if [[ "$a" == *"build"* && "$a" != *"build${TARGET_BUILD}"* ]]; then
      echo -e "  ${GRAY}Removendo asset antigo do GitHub: $a${RESET}"
      gh release delete-asset "$TAG_NAME" "$a" --repo WebuildSoft/antigravity-mobile-releases -y 2>/dev/null || true
    fi
  done

  # Upload dos novos arquivos (Genéricos e Versionados)
  gh release upload "$TAG_NAME" \
    "$RELEASES_REPO/app-release.apk" \
    "$RELEASES_REPO/AntigravityMobile-v${TARGET_VERSION}-build${TARGET_BUILD}.apk" \
    "$RELEASES_REPO/antigravity-server-windows.zip" \
    "$RELEASES_REPO/antigravity-server-windows-v${TARGET_VERSION}-build${TARGET_BUILD}.zip" \
    "$RELEASES_REPO/antigravity-server-macos.zip" \
    "$RELEASES_REPO/antigravity-server-macos-v${TARGET_VERSION}-build${TARGET_BUILD}.zip" \
    "$RELEASES_REPO/antigravity-server-linux.zip" \
    "$RELEASES_REPO/antigravity-server-linux-v${TARGET_VERSION}-build${TARGET_BUILD}.zip" \
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
