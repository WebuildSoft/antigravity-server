$ErrorActionPreference = "Continue"

$geminiDir = Join-Path $env:USERPROFILE ".gemini"
$profilesDir = Join-Path $geminiDir "profiles"
$credsFile = Join-Path $geminiDir "oauth_creds.json"
$accountsFile = Join-Path $geminiDir "google_accounts.json"
$port = if ($env:PORT) { $env:PORT } else { "4040" }

if (!(Test-Path $profilesDir)) {
    New-Item -ItemType Directory -Force -Path $profilesDir | Out-Null
}

Write-Host ""
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host "  🔐 Antigravity - Assistente de Conexao de Nova Conta" -ForegroundColor Green
Write-Host "==========================================================" -ForegroundColor Cyan
Write-Host ""

function Get-EmailFromCreds($path) {
    if (Test-Path $path) {
        try {
            $json = Get-Content -Raw -Path $path | ConvertFrom-Json
            if ($json.id_token) {
                $parts = $json.id_token.Split('.')
                if ($parts.Length -ge 2) {
                    $payload = $parts[1]
                    $mod = $payload.Length % 4
                    if ($mod -gt 0) { $payload += "=" * (4 - $mod) }
                    $bytes = [System.Convert]::FromBase64String($payload)
                    $decoded = [System.Text.Encoding]::UTF8.GetString($bytes) | ConvertFrom-Json
                    if ($decoded.email) { return $decoded.email }
                }
            }
            if ($json.email) { return $json.email }
        } catch {}
    }
    return $null
}

if (Test-Path $credsFile) {
    $currentEmail = Get-EmailFromCreds $credsFile
    if ($currentEmail) {
        Write-Host "Salvando backup da conta atual ($currentEmail)..." -ForegroundColor Yellow
        $profileDir = Join-Path $profilesDir $currentEmail
        if (!(Test-Path $profileDir)) { New-Item -ItemType Directory -Force -Path $profileDir | Out-Null }
        Copy-Item -Force $credsFile (Join-Path $profileDir "oauth_creds.json")
    }
    Move-Item -Force $credsFile "$credsFile.bak"
}

Write-Host "Uma janela do navegador sera aberta pelo Antigravity CLI oficial." -ForegroundColor Cyan
Write-Host "Faca login com a NOVA CONTA GOOGLE que deseja adicionar." -ForegroundColor Cyan
Write-Host "Depois de autorizar no navegador, digite 'exit' no terminal para concluir." -ForegroundColor Cyan
Write-Host "----------------------------------------------------------"

& agy

if (Test-Path $credsFile) {
    $newEmail = Get-EmailFromCreds $credsFile
    if ($newEmail) {
        Write-Host ""
        Write-Host "Sucesso! Nova conta autenticada: $newEmail" -ForegroundColor Green
        $newProfileDir = Join-Path $profilesDir $newEmail
        if (!(Test-Path $newProfileDir)) { New-Item -ItemType Directory -Force -Path $newProfileDir | Out-Null }
        Copy-Item -Force $credsFile (Join-Path $newProfileDir "oauth_creds.json")

        try {
            $body = @{ email = $newEmail } | ConvertTo-Json
            Invoke-RestMethod -Uri "http://localhost:$port/api/accounts/switch" -Method Post -ContentType "application/json" -Body $body | Out-Null
        } catch {}

        Write-Host "Conta $newEmail registrada nos perfis!" -ForegroundColor Green
        Write-Host "Ela ja esta disponivel no seu app mobile para alternar com 1 toque." -ForegroundColor Green
        Remove-Item -Force "$credsFile.bak" -ErrorAction SilentlyContinue
        Write-Host ""
        Write-Host "Pressione qualquer tecla para fechar..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 0
    }
}

if (Test-Path "$credsFile.bak") {
    Write-Host "Login cancelado. Restaurando conta anterior..." -ForegroundColor Yellow
    Move-Item -Force "$credsFile.bak" $credsFile
}

Write-Host "Nenhuma nova conta foi configurada." -ForegroundColor Red
Write-Host ""
Write-Host "Pressione qualquer tecla para fechar..."
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
