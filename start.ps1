# ==============================================================================
#  Antigravity Server - Inicializador para Windows com Tunel e QR Code
# ==============================================================================
$ErrorActionPreference = "Continue"

$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = Get-Location
}
Set-Location $scriptDir

# Carregar variaveis de ambiente do .env se existir
if (Test-Path "$scriptDir\.env") {
    $envLines = Get-Content "$scriptDir\.env" -ErrorAction SilentlyContinue
    foreach ($line in $envLines) {
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith("#") -and $trimmed.Contains("=")) {
            $parts = $trimmed.Split("=", 2)
            $k = $parts[0].Trim()
            $v = $parts[1].Trim()
            if ($v.StartsWith('"') -and $v.EndsWith('"')) { $v = $v.Substring(1, $v.Length - 2) }
            if ($v.StartsWith("'") -and $v.EndsWith("'")) { $v = $v.Substring(1, $v.Length - 2) }
            if (-not [System.Environment]::GetEnvironmentVariable($k)) {
                [System.Environment]::SetEnvironmentVariable($k, $v, "Process")
            }
        }
    }
}

$port = 4040
if ($env:PORT) {
    $port = [int]$env:PORT
}

function Test-PortInUse($p) {
    $conn = Get-NetTCPConnection -LocalPort $p -ErrorAction SilentlyContinue
    return ($null -ne $conn)
}

$originalPort = $port
while (Test-PortInUse $port) {
    Write-Host "[!] Porta $port ocupada por outro processo. Procurando proxima porta livre..." -ForegroundColor Yellow
    $port++
}

if ($port -ne $originalPort) {
    Write-Host "[OK] Porta livre selecionada automaticamente: $port" -ForegroundColor Green
}
$env:PORT = "$port"

# Localizar cloudflared
$cfBin = "$scriptDir\bin\cloudflared.exe"
if (-not (Test-Path $cfBin)) {
    if (Get-Command cloudflared.exe -ErrorAction SilentlyContinue) {
        $cfBin = (Get-Command cloudflared.exe).Source
    }
}

$cfProcess = $null
$serverProcess = $null

function Cleanup {
    Write-Host "`nEncerrando servidor e tunel..." -ForegroundColor Yellow
    if ($serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if ($cfProcess) {
        try {
            if (-not $cfProcess.HasExited) {
                $cfProcess.Kill()
            }
        } catch {}
    }
}

# ------------------------------------------------------------------------------
# Conectividade Remota: Quick Tunnel Cloudflare
# ------------------------------------------------------------------------------
$publicUrl = $null
if (Test-Path $cfBin) {
    Write-Host "Iniciando Quick Tunnel Cloudflare (Zero-Config)..." -ForegroundColor Cyan
    $tunnelLog = Join-Path $env:TEMP "cf_quick_tunnel_$port.log"
    if (Test-Path $tunnelLog) {
        Remove-Item $tunnelLog -Force -ErrorAction SilentlyContinue
    }

    try {
        $pinfo = New-Object System.Diagnostics.ProcessStartInfo
        $pinfo.FileName = $cfBin
        $pinfo.Arguments = "tunnel --url http://127.0.0.1:$port --logfile `"$tunnelLog`""
        $pinfo.UseShellExecute = $false
        $pinfo.CreateNoWindow = $true
        $cfProcess = [System.Diagnostics.Process]::Start($pinfo)
    } catch {
        Write-Host "[!] Falha ao iniciar cloudflared: $_" -ForegroundColor Red
    }

    Write-Host -NoNewline "Aguardando URL publica do tunel" -ForegroundColor Yellow
    for ($i = 0; $i -lt 30; $i++) {
        Start-Sleep -Seconds 1
        Write-Host -NoNewline "." -ForegroundColor Yellow
        if ($cfProcess -and $cfProcess.HasExited) {
            Write-Host "`n[!] O processo do tunel encerrou inesperadamente (ExitCode: $($cfProcess.ExitCode))." -ForegroundColor Red
            break
        }
        if (Test-Path $tunnelLog) {
            $logContent = Get-Content $tunnelLog -Raw -ErrorAction SilentlyContinue
            if ($logContent -match 'https://[-a-zA-Z0-9.]+\.trycloudflare\.com') {
                $publicUrl = $Matches[0]
                $env:PUBLIC_URL = $publicUrl
                break
            }
        }
    }
    Write-Host ""

    if ($publicUrl) {
        Write-Host "[OK] Tunel publico estabelecido: " -ForegroundColor Green -NoNewline
        Write-Host $publicUrl -ForegroundColor Cyan
    } else {
        Write-Host "[X] O tunel Cloudflare nao conseguiu estabelecer conexao." -ForegroundColor Red
        if (Test-Path $tunnelLog) {
            Write-Host "`n--- LOG DE ERRO DO TUNEL (cloudflared) ---" -ForegroundColor Yellow
            Get-Content $tunnelLog -ErrorAction SilentlyContinue | Select-Object -Last 12 | ForEach-Object {
                Write-Host "  $_" -ForegroundColor DarkYellow
            }
            Write-Host "-------------------------------------------`n" -ForegroundColor Yellow
        }
    }
}

# ------------------------------------------------------------------------------
# Iniciar Servidor Executavel
# ------------------------------------------------------------------------------
$serverExe = "$scriptDir\antigravity-server.exe"
if (-not (Test-Path $serverExe)) {
    $prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"
    if (Test-Path $prebuilt) {
        Copy-Item $prebuilt $serverExe -Force
    } else {
        Write-Host "[X] Executavel antigravity-server.exe nao encontrado." -ForegroundColor Red
        Write-Host "Por favor execute .\install.ps1 primeiro." -ForegroundColor Yellow
        Cleanup
        exit 1
    }
}

try {
    # Executar o servidor no console em primeiro plano
    & $serverExe
} finally {
    Cleanup
}
