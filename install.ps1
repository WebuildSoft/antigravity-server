# ==============================================================================
#  Antigravity Server - Instalador Inteligente para Windows (PowerShell)
# ==============================================================================
$ErrorActionPreference = "Stop"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "    INSTALADOR DO SERVIDOR ANTIGRAVITY MOBILE (WINDOWS)     " -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# 1. Definir diretorio de instalacao
$scriptDir = $PSScriptRoot
if (-not $scriptDir -or -not (Test-Path "$scriptDir\start.ps1")) {
    $scriptDir = Join-Path $HOME "antigravity-server"
}

if (-not (Test-Path $scriptDir)) {
    New-Item -ItemType Directory -Force -Path $scriptDir | Out-Null
}
Set-Location $scriptDir

# Criar pastas locais
New-Item -ItemType Directory -Force -Path "$scriptDir\bin", "$scriptDir\data", "$scriptDir\uploads" | Out-Null

# ------------------------------------------------------------------------------
# 2. Verificar ou Instalar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
Write-Host "`n[1/4] Verificando Antigravity CLI ('agy')..." -ForegroundColor Cyan
$agyPath = $null
if (Get-Command agy.exe -ErrorAction SilentlyContinue) {
    $agyPath = (Get-Command agy.exe).Source
} elseif (Get-Command agy -ErrorAction SilentlyContinue) {
    $agyPath = (Get-Command agy).Source
} else {
    $candidates = @(
        "$env:LOCALAPPDATA\agy\bin\agy.exe",
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

if (-not $agyPath) {
    Write-Host "      [!] Antigravity CLI ('agy') nao encontrado no sistema." -ForegroundColor Yellow
    Write-Host "      Baixando e instalando o Antigravity CLI ('agy') automaticamente..." -ForegroundColor Cyan
    try {
        $installerScript = Invoke-RestMethod -Uri "https://antigravity.google/cli/install.ps1"
        Invoke-Expression $installerScript 2>$null
        $installedAgy = Join-Path $env:LOCALAPPDATA "agy\bin\agy.exe"
        if (Test-Path $installedAgy) {
            $agyPath = $installedAgy
            $agyDir = Split-Path $agyPath
            if ($env:PATH -notlike "*$agyDir*") {
                $env:PATH = "$agyDir;$env:PATH"
            }
            Write-Host "      [OK] agy instalado com sucesso em: $agyPath" -ForegroundColor Green
        }
    } catch {
        Write-Host "      [!] Tentativa de auto-instalacao do agy: $_" -ForegroundColor Yellow
    }
}

if ($agyPath) {
    Write-Host "      [OK] agy encontrado: $agyPath" -ForegroundColor Green

    # --------------------------------------------------------------------------
    # Verificar Autenticacao Google no 'agy'
    # --------------------------------------------------------------------------
    Write-Host "`nVerificando autenticacao Google no Antigravity..." -ForegroundColor Cyan
    $authOk = $false
    try {
        $modelsLog = Join-Path $env:TEMP "agy_models.txt"
        $p = Start-Process -FilePath $agyPath -ArgumentList "models" -NoNewWindow -Wait -PassThru -RedirectStandardOutput $modelsLog -ErrorAction SilentlyContinue
        if ($p.ExitCode -eq 0) {
            $authOk = $true
        }
    } catch {}

    if ($authOk) {
        Write-Host "      [OK] Conta Google conectada e pronta para responder." -ForegroundColor Green
    } else {
        Write-Host "      [!] Nenhuma conta Google conectada ao Antigravity ainda." -ForegroundColor Yellow
        Write-Host "      Abrindo login Google no seu navegador..." -ForegroundColor Cyan
        try {
            Start-Process -FilePath $agyPath -Wait
            try {
                $p2 = Start-Process -FilePath $agyPath -ArgumentList "models" -NoNewWindow -Wait -PassThru -ErrorAction SilentlyContinue
                if ($p2.ExitCode -eq 0) {
                    Write-Host "      [OK] Login realizado com sucesso!" -ForegroundColor Green
                }
            } catch {}
        } catch {
            Write-Host "      [!] Autenticacao interativa ignorada: $_" -ForegroundColor Yellow
        }
    }
} else {
    Write-Host "      [!] Antigravity CLI ('agy') nao detectado." -ForegroundColor Yellow
}

# ------------------------------------------------------------------------------
# 3. Baixar ou Verificar Cloudflare Tunnel (cloudflared.exe)
# ------------------------------------------------------------------------------
Write-Host "`n[2/4] Verificando Cloudflare Tunnel (cloudflared)..." -ForegroundColor Cyan
$cfPath = "$scriptDir\bin\cloudflared.exe"
if (-not (Test-Path $cfPath)) {
    if (Get-Command cloudflared.exe -ErrorAction SilentlyContinue) {
        $sysCf = (Get-Command cloudflared.exe).Source
        Write-Host "      [OK] cloudflared do sistema encontrado: $sysCf" -ForegroundColor Green
        Copy-Item $sysCf $cfPath -Force
    } else {
        Write-Host "      [!] Baixando binario oficial seguro do Cloudflare Tunnel..." -ForegroundColor Yellow
        $cfUrl = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
        if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
            curl.exe -# -fSL "$cfUrl" -o "$cfPath"
        } else {
            Invoke-WebRequest -Uri $cfUrl -OutFile $cfPath -UseBasicParsing
        }
        Write-Host "      [OK] cloudflared baixado com sucesso em bin\cloudflared.exe" -ForegroundColor Green
    }
} else {
    Write-Host "      [OK] cloudflared local encontrado em: $cfPath" -ForegroundColor Green
}

# ------------------------------------------------------------------------------
# 4. Baixar / Atualizar Arquivos do Servidor (start.ps1, start.bat, executavel)
# ------------------------------------------------------------------------------
Write-Host "`n[3/4] Configurando arquivos e executavel do servidor..." -ForegroundColor Cyan

# Baixar start.ps1 e start.bat atualizados diretamente do repositorio oficial
$startPs1Url = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.ps1"
$startBatUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.bat"
$envExUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/.env.example"

Invoke-WebRequest -Uri $startPs1Url -OutFile "$scriptDir\start.ps1" -UseBasicParsing
Invoke-WebRequest -Uri $startBatUrl -OutFile "$scriptDir\start.bat" -UseBasicParsing

if (-not (Test-Path "$scriptDir\.env")) {
    if (Test-Path "$scriptDir\.env.example") {
        Copy-Item "$scriptDir\.env.example" "$scriptDir\.env"
    } else {
        Invoke-WebRequest -Uri $envExUrl -OutFile "$scriptDir\.env" -UseBasicParsing
    }
    Write-Host "      [OK] Arquivo .env configurado com sucesso" -ForegroundColor Green
}

$serverExe = "$scriptDir\antigravity-server.exe"
if (-not (Test-Path $serverExe)) {
    $prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"
    if (Test-Path $prebuilt) {
        Copy-Item $prebuilt $serverExe -Force
        Write-Host "      [OK] Utilizando executavel local" -ForegroundColor Green
    } else {
        Write-Host "      Baixando executavel oficial do servidor..." -ForegroundColor Cyan
        $repoExeUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-windows-amd64.exe"
        if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
            curl.exe -# -fSL "$repoExeUrl" -o "$serverExe"
        } else {
            Invoke-WebRequest -Uri $repoExeUrl -OutFile $serverExe -UseBasicParsing
        }
        Write-Host "      [OK] Executavel antigravity-server.exe pronto" -ForegroundColor Green
    }
} else {
    Write-Host "      [OK] Executavel antigravity-server.exe pronto" -ForegroundColor Green
}

Write-Host "`n============================================================" -ForegroundColor Green
Write-Host "    INSTALACAO NO WINDOWS CONCLUIDA COM SUCESSO!            " -ForegroundColor Green
Write-Host "============================================================" -ForegroundColor Green
Write-Host "Para iniciar o servidor no futuro a qualquer momento, execute:"
Write-Host "  cd `"$scriptDir`"; .\start.ps1" -ForegroundColor Cyan
Write-Host "  (ou de dois cliques em start.bat)`n" -ForegroundColor Cyan
Write-Host "Iniciando o servidor automaticamente agora...`n" -ForegroundColor Green

# Iniciar o servidor
& "$scriptDir\start.ps1"
