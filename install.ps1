# ==============================================================================
#  Antigravity Server - Instalador Inteligente para Windows (PowerShell)
# ==============================================================================
$ErrorActionPreference = "Stop"

$BOLD = "`e[1m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$CYAN = "`e[36m"
$RED = "`e[31m"
$RESET = "`e[0m"

Write-Host "${BOLD}${CYAN}"
Write-Host "============================================================"
Write-Host "    🚀 INSTALADOR DO SERVIDOR ANTIGRAVITY MOBILE (WINDOWS)   "
Write-Host "============================================================"
Write-Host "${RESET}"

# 1. Definir diretório de instalação
$scriptDir = $PSScriptRoot
if (-not $scriptDir -or -not (Test-Path "$scriptDir\go.mod")) {
    $scriptDir = Join-Path $HOME "antigravity-server"
    Write-Host "${CYAN}📦 Repositório não detectado na pasta atual.${RESET}"
    if (Test-Path "$scriptDir\.git") {
        Write-Host "${CYAN}🔄 Atualizando repositório existente em: $scriptDir...${RESET}"
        Push-Location $scriptDir
        try { git pull --quiet origin main } catch {}
        Pop-Location
    } elseif (Test-Path "$scriptDir\go.mod") {
        Write-Host "${GREEN}✓ Pasta do repositório já encontrada em: $scriptDir${RESET}"
    } else {
        Write-Host "${CYAN}⬇ Baixando repositório oficial em: $scriptDir...${RESET}"
        if (Get-Command git -ErrorAction SilentlyContinue) {
            git clone --quiet https://github.com/WebuildSoft/antigravity-server.git $scriptDir
        } else {
            Write-Host "${YELLOW}Git não encontrado. Baixando arquivos compactados...${RESET}"
            $zipPath = Join-Path $env:TEMP "antigravity-server.zip"
            Invoke-WebRequest -Uri "https://github.com/WebuildSoft/antigravity-server/archive/refs/heads/main.zip" -OutFile $zipPath
            Expand-Archive -Path $zipPath -DestinationPath $env:TEMP -Force
            Remove-Item -Path $zipPath -Force -ErrorAction SilentlyContinue
            Move-Item -Path (Join-Path $env:TEMP "antigravity-server-main") -Destination $scriptDir -Force
        }
    }
}

Set-Location $scriptDir

# Criar pastas locais
New-Item -ItemType Directory -Force -Path "$scriptDir\bin", "$scriptDir\data", "$scriptDir\uploads" | Out-Null

# ------------------------------------------------------------------------------
# 2. Verificar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
Write-Host "`n${BOLD}[1/4] Verificando Antigravity CLI ('agy')...${RESET}"
$agyPath = $null
if (Get-Command agy.exe -ErrorAction SilentlyContinue) {
    $agyPath = (Get-Command agy.exe).Source
} elseif (Get-Command agy -ErrorAction SilentlyContinue) {
    $agyPath = (Get-Command agy).Source
} else {
    $candidates = @(
        "$HOME\.local\bin\agy.exe",
        "$HOME\.gemini\antigravity-cli\bin\agy.exe",
        "$env:LOCALAPPDATA\Programs\antigravity\bin\agy.exe",
        "$env:LOCALAPPDATA\antigravity-cli\bin\agy.exe"
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) {
            $agyPath = $c
            $agyDir = Split-Path $c
            if ($env:PATH -notlike "*$agyDir*") {
                $env:PATH = "$agyDir;$env:PATH"
            }
            break
        }
    }
}

if ($agyPath) {
    Write-Host "      ${GREEN}✓ agy encontrado:${RESET} $agyPath"
} else {
    Write-Host "      ${YELLOW}! Antigravity CLI ('agy') não encontrado no PATH.${RESET}"
    Write-Host "      ${CYAN}💡 Se você ainda não instalou o agy no Windows, certifique-se de instalar o Antigravity CLI.${RESET}"
}

# ------------------------------------------------------------------------------
# 3. Baixar ou Verificar Cloudflare Tunnel (cloudflared.exe)
# ------------------------------------------------------------------------------
Write-Host "`n${BOLD}[2/4] Verificando Cloudflare Tunnel (cloudflared)...${RESET}"
$cfPath = "$scriptDir\bin\cloudflared.exe"
if (-not (Test-Path $cfPath)) {
    if (Get-Command cloudflared.exe -ErrorAction SilentlyContinue) {
        $sysCf = (Get-Command cloudflared.exe).Source
        Write-Host "      ${GREEN}✓ cloudflared do sistema encontrado:${RESET} $sysCf"
        Copy-Item $sysCf $cfPath -Force
    } else {
        Write-Host "      ${YELLOW}! Baixando binário oficial seguro do Cloudflare Tunnel...${RESET}"
        $cfUrl = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
        Invoke-WebRequest -Uri $cfUrl -OutFile $cfPath -UseBasicParsing
        Write-Host "      ${GREEN}✓ cloudflared baixado com sucesso em bin\cloudflared.exe${RESET}"
    }
} else {
    Write-Host "      ${GREEN}✓ cloudflared local encontrado em:${RESET} $cfPath"
}

# ------------------------------------------------------------------------------
# 4. Configurar Arquivo .env e Binário Executável
# ------------------------------------------------------------------------------
Write-Host "`n${BOLD}[3/4] Configurando arquivos e executável...${RESET}"
if (-not (Test-Path "$scriptDir\.env") -and (Test-Path "$scriptDir\.env.example")) {
    Copy-Item "$scriptDir\.env.example" "$scriptDir\.env"
    Write-Host "      ${GREEN}✓ Arquivo .env criado a partir de .env.example${RESET}"
}

$serverExe = "$scriptDir\antigravity-server.exe"
if (-not (Test-Path $serverExe)) {
    $prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"
    if (Test-Path $prebuilt) {
        Write-Host "      ${GREEN}✓ Utilizando binário pré-compilado para Windows x64${RESET}"
        Copy-Item $prebuilt $serverExe -Force
    } elseif (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host "      ${CYAN}Compilando antigravity-server via Go local...${RESET}"
        go build -ldflags="-s -w" -o $serverExe .
    } else {
        Write-Host "      ${CYAN}Baixando executável do servidor direto do repositório...${RESET}"
        $repoExeUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-windows-amd64.exe"
        Invoke-WebRequest -Uri $repoExeUrl -OutFile $serverExe -UseBasicParsing
    }
}

Write-Host "`n${BOLD}${GREEN}============================================================"
Write-Host "    🎉 INSTALAÇÃO NO WINDOWS CONCLUÍDA COM SUCESSO!         "
Write-Host "============================================================${RESET}"
Write-Host "Para iniciar o servidor no futuro a qualquer momento, execute:"
Write-Host "  ${CYAN}cd `"$scriptDir`"; .\start.ps1${RESET}"
Write-Host "  ${CYAN}(ou dê dois cliques em start.bat)${RESET}`n"
Write-Host "${BOLD}${GREEN}🚀 Iniciando o servidor automaticamente agora...${RESET}`n"

# Iniciar o servidor
& "$scriptDir\start.ps1"
