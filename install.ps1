# ==============================================================================
#  Antigravity Server - Instalador Inteligente para Windows (PowerShell)
# ==============================================================================
$ErrorActionPreference = "Stop"

Write-Host "============================================================" -ForegroundColor Cyan
Write-Host "    INSTALADOR DO SERVIDOR ANTIGRAVITY MOBILE (WINDOWS)     " -ForegroundColor Cyan
Write-Host "============================================================" -ForegroundColor Cyan

# 1. Definir diretorio de instalacao
$scriptDir = $PSScriptRoot
if (-not $scriptDir -or -not (Test-Path "$scriptDir\go.mod")) {
    $scriptDir = Join-Path $HOME "antigravity-server"
    Write-Host "Repositorio nao detectado na pasta atual." -ForegroundColor Cyan
    if (Test-Path "$scriptDir\.git") {
        Write-Host "Atualizando repositorio existente em: $scriptDir..." -ForegroundColor Cyan
        Push-Location $scriptDir
        try { git pull --quiet origin main } catch {}
        Pop-Location
    } elseif (Test-Path "$scriptDir\go.mod") {
        Write-Host "[OK] Pasta do repositorio ja encontrada em: $scriptDir" -ForegroundColor Green
    } else {
        Write-Host "Baixando repositorio oficial em: $scriptDir..." -ForegroundColor Cyan
        if (Get-Command git -ErrorAction SilentlyContinue) {
            git clone --quiet https://github.com/WebuildSoft/antigravity-server.git $scriptDir
        } else {
            Write-Host "Git nao encontrado. Baixando arquivos compactados..." -ForegroundColor Yellow
            $zipPath = Join-Path $env:TEMP "antigravity-server.zip"
            if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
                curl.exe -# -fSL "https://github.com/WebuildSoft/antigravity-server/archive/refs/heads/main.zip" -o "$zipPath"
            } else {
                Invoke-WebRequest -Uri "https://github.com/WebuildSoft/antigravity-server/archive/refs/heads/main.zip" -OutFile "$zipPath"
            }
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
# 4. Configurar Arquivo .env e Binario Executavel
# ------------------------------------------------------------------------------
Write-Host "`n[3/4] Configurando arquivos e executavel..." -ForegroundColor Cyan
if (-not (Test-Path "$scriptDir\.env") -and (Test-Path "$scriptDir\.env.example")) {
    Copy-Item "$scriptDir\.env.example" "$scriptDir\.env"
    Write-Host "      [OK] Arquivo .env criado a partir de .env.example" -ForegroundColor Green
}

$serverExe = "$scriptDir\antigravity-server.exe"
if (-not (Test-Path $serverExe)) {
    $prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"
    if (Test-Path $prebuilt) {
        Write-Host "      [OK] Utilizando binario pre-compilado para Windows x64" -ForegroundColor Green
        Copy-Item $prebuilt $serverExe -Force
    } elseif (Get-Command go -ErrorAction SilentlyContinue) {
        Write-Host "      Compilando antigravity-server via Go local..." -ForegroundColor Cyan
        go build -ldflags="-s -w" -o $serverExe .
    } else {
        Write-Host "      Baixando executavel do servidor direto do repositorio..." -ForegroundColor Cyan
        $repoExeUrl = "https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/bin/antigravity-server-windows-amd64.exe"
        if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
            curl.exe -# -fSL "$repoExeUrl" -o "$serverExe"
        } else {
            Invoke-WebRequest -Uri $repoExeUrl -OutFile $serverExe -UseBasicParsing
        }
    }
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
