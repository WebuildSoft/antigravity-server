# ==============================================================================
#  Antigravity Server - Inicializador para Windows com Túnel e QR Code
# ==============================================================================
$ErrorActionPreference = "Continue"

$BOLD = "`e[1m"
$GREEN = "`e[32m"
$YELLOW = "`e[33m"
$CYAN = "`e[36m"
$RED = "`e[31m"
$RESET = "`e[0m"

$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = Get-Location
}
Set-Location $scriptDir

# Carregar variáveis de ambiente do .env se existir
if (Test-Path "$scriptDir\.env") {
    Get-Content "$scriptDir\.env" | ForEach-Object {
        $line = $_.Trim()
        if ($line -and -not $line.StartsWith("#") -and $line.Contains("=")) {
            $parts = $line.Split("=", 2)
            $k = $parts[0].Trim()
            $v = $parts[1].Trim().Trim('"').Trim("'")
            if (-not [System.Environment]::GetEnvironmentVariable($k)) {
                [System.Environment]::SetEnvironmentVariable($k, $v, "Process")
            }
        }
    }
}

$port = if ($env:PORT) { [int]$env:PORT } else { 4040 }

function Test-PortInUse($p) {
    $conn = Get-NetTCPConnection -LocalPort $p -ErrorAction SilentlyContinue
    return ($null -ne $conn)
}

$originalPort = $port
while (Test-PortInUse $port) {
    Write-Host "${YELLOW}⚠️ Porta $port ocupada por outro processo. Procurando próxima porta livre...${RESET}"
    $port++
}

if ($port -ne $originalPort) {
    Write-Host "${GREEN}✓ Porta livre selecionada automaticamente:${RESET} ${BOLD}${port}${RESET}"
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
    Write-Host "`n${YELLOW}Encerrando servidor e túnel...${RESET}"
    if ($serverProcess -and -not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force -ErrorAction SilentlyContinue
    }
    if ($cfProcess -and -not $cfProcess.HasExited) {
        Stop-Process -Id $cfProcess.Id -Force -ErrorAction SilentlyContinue
    }
}

# ------------------------------------------------------------------------------
# Conectividade Remota: Quick Tunnel Cloudflare
# ------------------------------------------------------------------------------
$publicUrl = $null
if (Test-Path $cfBin) {
    Write-Host "${CYAN}Iniciando Quick Tunnel gratuito Cloudflare (Zero-Config)...${RESET}"
    $tunnelLog = Join-Path $env:TEMP "cf_quick_tunnel_$port.log"
    if (Test-Path $tunnelLog) { Remove-Item $tunnelLog -Force -ErrorAction SilentlyContinue }

    $cfProcess = Start-Process -FilePath $cfBin -ArgumentList "tunnel", "--url", "http://localhost:$port" -RedirectStandardError $tunnelLog -NoNewWindow -PassThru

    Write-Host -NoNewline "Aguardando URL pública do túnel"
    for ($i = 0; $i -lt 25; $i++) {
        Start-Sleep -Seconds 1
        Write-Host -NoNewline "."
        if (Test-Path $tunnelLog) {
            $content = Get-Content $tunnelLog -Raw -ErrorAction SilentlyContinue
            if ($content -match "https://[-a-zA-Z0-9\.]+\.trycloudflare\.com") {
                $publicUrl = $Matches[0]
                $env:PUBLIC_URL = $publicUrl
                break
            }
        }
    }
    Write-Host ""

    if ($publicUrl) {
        Write-Host "${GREEN}✓ Túnel público estabelecido:${RESET} ${BOLD}${publicUrl}${RESET}"
    } else {
        Write-Host "${YELLOW}⚠️ O túnel demorou a responder. O servidor iniciará apenas com os links locais/IP.${RESET}"
    }
}

# ------------------------------------------------------------------------------
# Iniciar Servidor Executável
# ------------------------------------------------------------------------------
$serverExe = "$scriptDir\antigravity-server.exe"
if (-not (Test-Path $serverExe)) {
    $prebuilt = "$scriptDir\bin\antigravity-server-windows-amd64.exe"
    if (Test-Path $prebuilt) {
        Copy-Item $prebuilt $serverExe -Force
    } else {
        Write-Host "${RED}❌ Executável antigravity-server.exe não encontrado.${RESET}"
        Write-Host "Por favor execute .\install.ps1 primeiro."
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
