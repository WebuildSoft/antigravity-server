# ==============================================================================
#  Antigravity Server - Instalador Inteligente para Windows (PowerShell)
# ==============================================================================
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force -ErrorAction SilentlyContinue
try { [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.SecurityProtocolType]::Tls12 -bor 3072 } catch {}

$ErrorActionPreference = "Stop"

function Print-Bullet($label, $value, $color = "Green") {
    Write-Host "  " -NoNewline
    Write-Host "● " -ForegroundColor $color -NoNewline
    Write-Host ("{0,-16}" -f $label) -NoNewline
    Write-Host $value
}

function Safe-Download($url, $dest, $useCacheBust = $false) {
    $targetUrl = $url
    if ($useCacheBust) {
        $targetUrl = "$url?v=" + [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    }

    $downloaded = $false
    try {
        $wc = New-Object System.Net.WebClient
        $wc.Headers.Add("User-Agent", "AntigravityInstaller/1.4.0 (Windows)")
        $wc.DownloadFile($targetUrl, $dest)
        $downloaded = $true
    } catch {}

    if (-not $downloaded) {
        try {
            Invoke-WebRequest -Uri $targetUrl -OutFile $dest -UseBasicParsing -TimeoutSec 30
            $downloaded = $true
        } catch {}
    }

    if (-not $downloaded -and (Get-Command curl.exe -ErrorAction SilentlyContinue)) {
        & curl.exe -fsSL "$url" -o "$dest"
    }
}

Write-Host ""
Write-Host "╭─────────────────────────────────────────────────────────────╮" -ForegroundColor Cyan
Write-Host "│  ANTIGRAVITY SERVER • INSTALADOR (WINDOWS)                  │" -ForegroundColor Cyan
Write-Host "│  WebuildSoft Engine • Configuracao Automatizada             │" -ForegroundColor DarkGray
Write-Host "╰─────────────────────────────────────────────────────────────╯" -ForegroundColor Cyan
Write-Host ""

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

$arch = if ([System.Environment]::Is64BitOperatingSystem) { "x86_64" } else { "x86" }
Print-Bullet "Sistema:" "Windows ($arch)" "Green"

# ------------------------------------------------------------------------------
# 2. Verificar ou Instalar Antigravity CLI ('agy')
# ------------------------------------------------------------------------------
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
        }
    } catch {}
}

if ($agyPath) {
    $agyVer = "instalado"
    try {
        $vOutput = & $agyPath --version 2>$null
        if ($vOutput) { $agyVer = $vOutput.Trim() }
    } catch {}
    Print-Bullet "CLI Agente:" "Conectado ($agyVer)" "Green"

    # Verificar Autenticacao Google no 'agy'
    $authOk = $false
    try {
        $modelsLog = Join-Path $env:TEMP "agy_models.txt"
        $modelsErr = Join-Path $env:TEMP "agy_models_err.txt"
        $p = Start-Process -FilePath $agyPath -ArgumentList "models" -NoNewWindow -Wait -PassThru -RedirectStandardOutput $modelsLog -RedirectStandardError $modelsErr -ErrorAction SilentlyContinue
        if ($p.ExitCode -eq 0) {
            $authOk = $true
        }
    } catch {}

    if ($authOk) {
        Print-Bullet "Autenticação:" "Conta Google vinculada" "Green"
    } else {
        Print-Bullet "Autenticação:" "Pendente (conecte via 'agy')" "Yellow"
    }
} else {
    Print-Bullet "CLI Agente:" "Não detectado" "Yellow"
}

# ------------------------------------------------------------------------------
# 3. Baixar ou Verificar Cloudflare Tunnel (cloudflared.exe)
# ------------------------------------------------------------------------------
$cfPath = "$scriptDir\bin\cloudflared.exe"
if (-not (Test-Path $cfPath)) {
    if (Get-Command cloudflared.exe -ErrorAction SilentlyContinue) {
        $sysCf = (Get-Command cloudflared.exe).Source
        Copy-Item $sysCf $cfPath -Force
        Print-Bullet "Túnel Seguro:" "Cloudflare Zero Trust pronto" "Green"
    } else {
        $cfUrl = "https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe"
        Safe-Download $cfUrl $cfPath $false
        Unblock-File -Path "$cfPath" -ErrorAction SilentlyContinue
        Print-Bullet "Túnel Seguro:" "Cloudflare Zero Trust configurado" "Green"
    }
} else {
    Unblock-File -Path "$cfPath" -ErrorAction SilentlyContinue
    Print-Bullet "Túnel Seguro:" "Cloudflare Zero Trust pronto" "Green"
}

# Finalizar instancia anterior do servidor se estiver em execucao
Get-Process antigravity-server -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

# ------------------------------------------------------------------------------
# 4. Baixar / Atualizar Arquivos do Servidor (start.ps1, start.bat, executavel, web)
# ------------------------------------------------------------------------------
$startPs1Url = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.ps1"
$startBatUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/start.bat"
$envExUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/.env.example"

if (Test-Path "$scriptDir\antigravity-server" -PathType Container) {
    if (Test-Path "$scriptDir\antigravity-server\web" -and -not (Test-Path "$scriptDir\web")) {
        Copy-Item -Path "$scriptDir\antigravity-server\web" -Destination $scriptDir -Recurse -Force
    }
    Remove-Item -Path "$scriptDir\antigravity-server" -Recurse -Force -ErrorAction SilentlyContinue
}

# Baixar pacote da interface web se nao existir
if (-not (Test-Path "$scriptDir\web\index.html")) {
    $pkgUrl = "https://github.com/WebuildSoft/antigravity-mobile-releases/releases/latest/download/antigravity-server-windows.zip"
    $pkgZip = Join-Path $env:TEMP "antigravity-server-windows.zip"
    $pkgExtract = Join-Path $env:TEMP "antigravity_pkg_extract"
    if (Test-Path $pkgExtract) { Remove-Item -Path $pkgExtract -Recurse -Force -ErrorAction SilentlyContinue }
    Safe-Download $pkgUrl $pkgZip $false
    Expand-Archive -Path $pkgZip -DestinationPath $pkgExtract -Force
    if (Test-Path "$pkgExtract\antigravity-server\web") {
        Copy-Item -Path "$pkgExtract\antigravity-server\web" -Destination $scriptDir -Recurse -Force
    } elseif (Test-Path "$pkgExtract\web") {
        Copy-Item -Path "$pkgExtract\web" -Destination $scriptDir -Recurse -Force
    }
    Remove-Item -Path $pkgZip, $pkgExtract -Recurse -Force -ErrorAction SilentlyContinue
}
Print-Bullet "Interface Web:" "Flutter Web client pronto" "Green"

Safe-Download $startPs1Url "$scriptDir\start.ps1" $true
Safe-Download $startBatUrl "$scriptDir\start.bat" $true
Unblock-File -Path "$scriptDir\start.ps1" -ErrorAction SilentlyContinue
Unblock-File -Path "$scriptDir\start.bat" -ErrorAction SilentlyContinue

if (-not (Test-Path "$scriptDir\.env")) {
    if (Test-Path "$scriptDir\.env.example") {
        Copy-Item "$scriptDir\.env.example" "$scriptDir\.env"
    } else {
        Safe-Download $envExUrl "$scriptDir\.env" $false
    }
}

$serverExe = "$scriptDir\antigravity-server.exe"
$repoExeUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-windows-amd64.exe"
$prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"

if ((Test-Path $prebuilt) -and (Test-Path "$scriptDir\main.go")) {
    try { Copy-Item $prebuilt $serverExe -Force -ErrorAction SilentlyContinue } catch {}
} else {
    Safe-Download $repoExeUrl $serverExe $true
}
Unblock-File -Path "$serverExe" -ErrorAction SilentlyContinue
Print-Bullet "Executável:" "Servidor nativo atualizado" "Green"

Write-Host ""
Write-Host "  Iniciando servidor nativo..." -ForegroundColor DarkGray
Write-Host ""

# Iniciar o servidor
$psHost = if (Get-Command pwsh.exe -ErrorAction SilentlyContinue) { "pwsh.exe" } else { "powershell.exe" }
& $psHost -NoProfile -ExecutionPolicy Bypass -File "$scriptDir\start.ps1"
