package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"antigravity-bridge/config"
)

type AccountProfile struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Picture  string `json:"picture"`
	IsActive bool   `json:"isActive"`
	LastUsed int64  `json:"lastUsed"`
}

type GoogleAccountsState struct {
	Active string   `json:"active"`
	Old    []string `json:"old"`
}

var (
	accountMutex sync.Mutex
)

func getGeminiDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.Getenv("HOME")
		if home == "" {
			home = "."
		}
	}
	return filepath.Join(home, ".gemini")
}

func getProfilesDir() string {
	return filepath.Join(getGeminiDir(), "profiles")
}

func parseJWTClaims(jwtStr string) (map[string]interface{}, error) {
	parts := strings.Split(jwtStr, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("formato de JWT inválido")
	}
	payloadB64 := parts[1]
	if m := len(payloadB64) % 4; m != 0 {
		payloadB64 += strings.Repeat("=", 4-m)
	}
	data, err := base64.URLEncoding.DecodeString(payloadB64)
	if err != nil {
		data, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, err
		}
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(data, &claims); err != nil {
		return nil, err
	}
	return claims, nil
}

func extractInfoFromCreds(credsPath string) (email, name, picture string, err error) {
	b, err := os.ReadFile(credsPath)
	if err != nil {
		return "", "", "", err
	}
	var creds map[string]interface{}
	if err := json.Unmarshal(b, &creds); err != nil {
		return "", "", "", err
	}

	if idToken, ok := creds["id_token"].(string); ok && idToken != "" {
		claims, err := parseJWTClaims(idToken)
		if err == nil {
			if e, ok := claims["email"].(string); ok {
				email = e
			}
			if n, ok := claims["name"].(string); ok {
				name = n
			}
			if p, ok := claims["picture"].(string); ok {
				picture = p
			}
		}
	}

	if email == "" {
		if e, ok := creds["email"].(string); ok {
			email = e
		}
	}

	if name == "" && email != "" {
		parts := strings.Split(email, "@")
		name = parts[0]
	}

	return email, name, picture, nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func syncOSKeyring(targetEmail, credsPath string) error {
	b, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("falha ao ler credenciais: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return fmt.Errorf("falha ao interpretar JSON: %v", err)
	}

	accessToken, _ := raw["access_token"].(string)
	refreshToken, _ := raw["refresh_token"].(string)
	tokenType, _ := raw["token_type"].(string)
	if tokenType == "" {
		tokenType = "Bearer"
	}

	expiresIn := 3599
	if exp, ok := raw["expires_in"].(float64); ok && exp > 0 {
		expiresIn = int(exp)
	}

	expiryStr := time.Now().Add(time.Duration(expiresIn) * time.Second).Format("2006-01-02T15:04:05.999999999-07:00")

	type TokenInfo struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		Expiry       string `json:"expiry"`
		ExpiresIn    int    `json:"expires_in"`
	}

	payload := struct {
		Token        TokenInfo `json:"token"`
		AuthMethod   string    `json:"auth_method"`
		IDTokenEmail string    `json:"id_token_email"`
	}{
		Token: TokenInfo{
			AccessToken:  accessToken,
			TokenType:    tokenType,
			RefreshToken: refreshToken,
			Expiry:       expiryStr,
			ExpiresIn:    expiresIn,
		},
		AuthMethod:   "consumer",
		IDTokenEmail: targetEmail,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	b64Val := base64.StdEncoding.EncodeToString(payloadBytes)
	keyringVal := "go-keyring-base64:" + b64Val

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("/usr/bin/security", "add-generic-password",
			"-a", "antigravity",
			"-s", "gemini",
			"-w", keyringVal,
			"-U",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[Accounts] ⚠️ Aviso ao gravar no macOS Keychain: %v (%s)\n", err, string(out))
			return err
		}
		fmt.Printf("[Accounts] 🔑 macOS Keychain atualizado com sucesso para: %s\n", targetEmail)

	case "windows":
		cmd := exec.Command("cmdkey", "/generic:gemini/antigravity", "/user:antigravity", "/pass:"+keyringVal)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[Accounts] ⚠️ Aviso ao gravar no Windows Credential Manager: %v (%s)\n", err, string(out))
			return err
		}
		fmt.Printf("[Accounts] 🔑 Windows Credential Manager atualizado com sucesso para: %s\n", targetEmail)

	case "linux":
		cmd := exec.Command("secret-tool", "store", "--label=Antigravity", "service", "gemini", "username", "antigravity")
		cmd.Stdin = strings.NewReader(keyringVal)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("[Accounts] ⚠️ Aviso ao gravar no Secret Service Linux: %v (%s)\n", err, string(out))
		} else {
			fmt.Printf("[Accounts] 🔑 Linux Secret Service atualizado para: %s\n", targetEmail)
		}
	}

	return nil
}

func getActiveGoogleAccountEmail() string {
	geminiDir := getGeminiDir()
	accountsFile := filepath.Join(geminiDir, "google_accounts.json")
	if b, err := os.ReadFile(accountsFile); err == nil {
		var state GoogleAccountsState
		if err := json.Unmarshal(b, &state); err == nil && state.Active != "" {
			return state.Active
		}
	}

	credsFile := filepath.Join(geminiDir, "oauth_creds.json")
	if email, _, _, err := extractInfoFromCreds(credsFile); err == nil && email != "" {
		return email
	}

	return ""
}

// InitAccountsManager initializes the profiles directory and syncs the current active account.
func InitAccountsManager() {
	accountMutex.Lock()
	defer accountMutex.Unlock()

	geminiDir := getGeminiDir()
	profilesDir := getProfilesDir()
	_ = os.MkdirAll(profilesDir, 0755)

	credsPath := filepath.Join(geminiDir, "oauth_creds.json")
	if _, err := os.Stat(credsPath); err == nil {
		email, name, picture, err := extractInfoFromCreds(credsPath)
		if err == nil && email != "" {
			profileDir := filepath.Join(profilesDir, email)
			_ = os.MkdirAll(profileDir, 0755)
			profileCreds := filepath.Join(profileDir, "oauth_creds.json")
			_ = copyFile(credsPath, profileCreds)

			info := AccountProfile{
				Email:    email,
				Name:     name,
				Picture:  picture,
				IsActive: true,
				LastUsed: time.Now().UnixMilli(),
			}
			infoBytes, _ := json.MarshalIndent(info, "", "  ")
			_ = os.WriteFile(filepath.Join(profileDir, "info.json"), infoBytes, 0644)
			fmt.Printf("[Accounts] Perfil inicial sincronizado: %s (%s)\n", name, email)
		}
	}
}

// ListAccounts returns all registered account profiles with the active one marked.
func ListAccounts() ([]AccountProfile, error) {
	accountMutex.Lock()
	defer accountMutex.Unlock()

	geminiDir := getGeminiDir()
	profilesDir := getProfilesDir()
	activeEmail := getActiveGoogleAccountEmail()

	// If current oauth_creds.json exists, update active profile's tokens
	credsPath := filepath.Join(geminiDir, "oauth_creds.json")
	if _, err := os.Stat(credsPath); err == nil {
		if email, name, picture, err := extractInfoFromCreds(credsPath); err == nil && email != "" {
			profileDir := filepath.Join(profilesDir, email)
			_ = os.MkdirAll(profileDir, 0755)
			_ = copyFile(credsPath, filepath.Join(profileDir, "oauth_creds.json"))
			info := AccountProfile{
				Email:    email,
				Name:     name,
				Picture:  picture,
				IsActive: (email == activeEmail),
				LastUsed: time.Now().UnixMilli(),
			}
			infoBytes, _ := json.MarshalIndent(info, "", "  ")
			_ = os.WriteFile(filepath.Join(profileDir, "info.json"), infoBytes, 0644)
		}
	}

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return []AccountProfile{}, nil
	}

	var list []AccountProfile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		email := entry.Name()
		profileDir := filepath.Join(profilesDir, email)
		profileCreds := filepath.Join(profileDir, "oauth_creds.json")
		if _, err := os.Stat(profileCreds); err != nil {
			continue
		}

		profile := AccountProfile{
			Email:    email,
			Name:     strings.Split(email, "@")[0],
			IsActive: (email == activeEmail),
		}

		infoFile := filepath.Join(profileDir, "info.json")
		if b, err := os.ReadFile(infoFile); err == nil {
			var saved AccountProfile
			if err := json.Unmarshal(b, &saved); err == nil {
				profile.Name = saved.Name
				profile.Picture = saved.Picture
				profile.LastUsed = saved.LastUsed
			}
		}

		if profile.Picture == "" {
			if _, n, p, err := extractInfoFromCreds(profileCreds); err == nil {
				if n != "" {
					profile.Name = n
				}
				profile.Picture = p
			}
		}

		list = append(list, profile)
	}

	// Sort active account first, then most recently used
	sort.Slice(list, func(i, j int) bool {
		if list[i].IsActive != list[j].IsActive {
			return list[i].IsActive
		}
		return list[i].LastUsed > list[j].LastUsed
	})

	return list, nil
}

// SwitchAccount switches the active Google account to the target email.
func SwitchAccount(targetEmail string) (*AccountProfile, error) {
	accountMutex.Lock()
	defer accountMutex.Unlock()

	targetEmail = strings.TrimSpace(targetEmail)
	if targetEmail == "" {
		return nil, fmt.Errorf("e-mail de destino não especificado")
	}

	geminiDir := getGeminiDir()
	profilesDir := getProfilesDir()
	targetCreds := filepath.Join(profilesDir, targetEmail, "oauth_creds.json")
	if _, err := os.Stat(targetCreds); os.IsNotExist(err) {
		return nil, fmt.Errorf("perfil da conta %s não encontrado no disco", targetEmail)
	}

	activeCreds := filepath.Join(geminiDir, "oauth_creds.json")
	// Backup current active credentials into its profile directory
	if currentEmail, _, _, err := extractInfoFromCreds(activeCreds); err == nil && currentEmail != "" && currentEmail != targetEmail {
		currentProfileDir := filepath.Join(profilesDir, currentEmail)
		_ = os.MkdirAll(currentProfileDir, 0755)
		_ = copyFile(activeCreds, filepath.Join(currentProfileDir, "oauth_creds.json"))
	}

	// Copy target credentials to active oauth_creds.json
	if err := copyFile(targetCreds, activeCreds); err != nil {
		return nil, fmt.Errorf("falha ao ativar credenciais: %v", err)
	}

	// Sincroniza o Keyring nativo do Sistema Operacional (macOS Keychain, Windows Credential Manager, Linux)
	if err := syncOSKeyring(targetEmail, targetCreds); err != nil {
		fmt.Printf("[Accounts] ⚠️ Falha ao sincronizar OS Keyring: %v\n", err)
	}

	// Update google_accounts.json
	accountsFile := filepath.Join(geminiDir, "google_accounts.json")
	var accountsState GoogleAccountsState
	if b, err := os.ReadFile(accountsFile); err == nil {
		_ = json.Unmarshal(b, &accountsState)
	}
	oldSet := make(map[string]bool)
	if accountsState.Active != "" && accountsState.Active != targetEmail {
		oldSet[accountsState.Active] = true
	}
	for _, o := range accountsState.Old {
		if o != targetEmail {
			oldSet[o] = true
		}
	}
	var newOld []string
	for o := range oldSet {
		newOld = append(newOld, o)
	}
	accountsState.Active = targetEmail
	accountsState.Old = newOld

	stateBytes, _ := json.MarshalIndent(accountsState, "", "  ")
	_ = os.WriteFile(accountsFile, stateBytes, 0644)

	// Update info.json for target
	targetInfoFile := filepath.Join(profilesDir, targetEmail, "info.json")
	var activeProfile AccountProfile
	if b, err := os.ReadFile(targetInfoFile); err == nil {
		_ = json.Unmarshal(b, &activeProfile)
	}
	activeProfile.Email = targetEmail
	activeProfile.IsActive = true
	activeProfile.LastUsed = time.Now().UnixMilli()
	if activeProfile.Name == "" {
		activeProfile.Name = strings.Split(targetEmail, "@")[0]
	}
	infoBytes, _ := json.MarshalIndent(activeProfile, "", "  ")
	_ = os.WriteFile(targetInfoFile, infoBytes, 0644)

	fmt.Printf("[Accounts] Conta ativada com sucesso: %s\n", targetEmail)

	// Invalidate quota cache synchronously so immediate queries do not see stale limits
	InvalidateQuotaCache()
	go func() {
		_, _ = fetchQuotaFromAgy()
	}()

	return &activeProfile, nil
}

// ImportAccount imports raw JSON credentials into profiles and activates them.
func ImportAccount(rawCreds []byte) (*AccountProfile, error) {
	var creds map[string]interface{}
	if err := json.Unmarshal(rawCreds, &creds); err != nil {
		return nil, fmt.Errorf("JSON de credenciais inválido: %v", err)
	}

	if _, hasAccess := creds["access_token"]; !hasAccess {
		if _, hasRefresh := creds["refresh_token"]; !hasRefresh {
			return nil, fmt.Errorf("credenciais inválidas: access_token ou refresh_token não encontrados")
		}
	}

	var email, name, picture string
	if idToken, ok := creds["id_token"].(string); ok && idToken != "" {
		claims, err := parseJWTClaims(idToken)
		if err == nil {
			if e, ok := claims["email"].(string); ok {
				email = e
			}
			if n, ok := claims["name"].(string); ok {
				name = n
			}
			if p, ok := claims["picture"].(string); ok {
				picture = p
			}
		}
	}

	if email == "" {
		if e, ok := creds["email"].(string); ok {
			email = e
		}
	}

	if email == "" {
		return nil, fmt.Errorf("não foi possível identificar o e-mail da conta no token")
	}

	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	profilesDir := getProfilesDir()
	profileDir := filepath.Join(profilesDir, email)
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return nil, err
	}

	profileCreds := filepath.Join(profileDir, "oauth_creds.json")
	if err := os.WriteFile(profileCreds, rawCreds, 0600); err != nil {
		return nil, err
	}

	profile := AccountProfile{
		Email:    email,
		Name:     name,
		Picture:  picture,
		IsActive: false,
		LastUsed: time.Now().UnixMilli(),
	}
	infoBytes, _ := json.MarshalIndent(profile, "", "  ")
	_ = os.WriteFile(filepath.Join(profileDir, "info.json"), infoBytes, 0644)

	// Automatically switch to the imported account
	return SwitchAccount(email)
}

// RemoveAccount deletes a non-active account profile.
func RemoveAccount(email string) error {
	accountMutex.Lock()
	defer accountMutex.Unlock()

	activeEmail := getActiveGoogleAccountEmail()
	if email == activeEmail {
		return fmt.Errorf("não é possível remover a conta atualmente ativa. Mude para outra conta antes de remover esta")
	}

	profilesDir := getProfilesDir()
	profileDir := filepath.Join(profilesDir, email)
	if err := os.RemoveAll(profileDir); err != nil {
		return err
	}

	// Update google_accounts.json
	geminiDir := getGeminiDir()
	accountsFile := filepath.Join(geminiDir, "google_accounts.json")
	if b, err := os.ReadFile(accountsFile); err == nil {
		var state GoogleAccountsState
		if err := json.Unmarshal(b, &state); err == nil {
			var newOld []string
			for _, o := range state.Old {
				if o != email {
					newOld = append(newOld, o)
				}
			}
			state.Old = newOld
			stateBytes, _ := json.MarshalIndent(state, "", "  ")
			_ = os.WriteFile(accountsFile, stateBytes, 0644)
		}
	}

	return nil
}

func getScriptsDir() string {
	base := filepath.Dir(config.AppConfig.VersionFile)
	if base == "" || base == "." {
		if exe, err := os.Executable(); err == nil {
			base = filepath.Dir(exe)
		}
	}
	dir := filepath.Join(base, "scripts")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

const shAddAccountScript = `#!/usr/bin/env bash
set -e

GEMINI_DIR="$HOME/.gemini"
PROFILES_DIR="$GEMINI_DIR/profiles"
CREDS_FILE="$GEMINI_DIR/oauth_creds.json"
ACCOUNTS_FILE="$GEMINI_DIR/google_accounts.json"
PORT="${PORT:-4040}"

mkdir -p "$PROFILES_DIR"

echo ""
echo "=========================================================="
echo "  🔐 Antigravity - Assistente de Conexão de Nova Conta"
echo "=========================================================="
echo ""

# Backup da conta atual se existir
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
" 2>/dev/null || true)
    if [ -n "$CURRENT_EMAIL" ]; then
        echo "💾 Salvando backup da conta atual ($CURRENT_EMAIL)..."
        mkdir -p "$PROFILES_DIR/$CURRENT_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$CURRENT_EMAIL/oauth_creds.json"
    fi
    mv "$CREDS_FILE" "$CREDS_FILE.bak"
fi

echo "👉 Uma janela do navegador será aberta pelo Antigravity CLI oficial."
echo "👉 Faça login com a NOVA CONTA GOOGLE que deseja adicionar."
echo "👉 Depois de autorizar no navegador, digite 'exit' no terminal para concluir."
echo "----------------------------------------------------------"

AGY_BIN="$(which agy 2>/dev/null || echo "$HOME/.local/bin/agy")"
if [ ! -x "$AGY_BIN" ]; then
    AGY_BIN="$HOME/.gemini/antigravity-cli/bin/agy"
fi

"$AGY_BIN" || true

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
" 2>/dev/null || true)
    if [ -n "$NEW_EMAIL" ]; then
        echo ""
        echo "🎉 Sucesso! Nova conta autenticada: $NEW_EMAIL"
        mkdir -p "$PROFILES_DIR/$NEW_EMAIL"
        cp "$CREDS_FILE" "$PROFILES_DIR/$NEW_EMAIL/oauth_creds.json"

        # Notifica o backend Go
        curl -s -X POST "http://localhost:${PORT}/api/accounts/switch" -H "Content-Type: application/json" -d "{\"email\": \"$NEW_EMAIL\"}" >/dev/null 2>&1 || true

        echo "✅ Conta $NEW_EMAIL registrada nos perfis!"
        echo "📱 Ela já está disponível no seu app mobile para alternar com 1 toque."
        rm -f "$CREDS_FILE.bak"
        echo ""
        read -p "Pressione Enter para fechar esta janela..."
        exit 0
    fi
fi

# Se falhou ou cancelou, restaura o backup
if [ -f "$CREDS_FILE.bak" ]; then
    echo "⚠️ Login cancelado ou não concluído. Restaurando conta anterior..."
    mv "$CREDS_FILE.bak" "$CREDS_FILE"
fi

echo "❌ Nenhuma nova conta foi configurada."
echo ""
read -p "Pressione Enter para fechar esta janela..."
`

const psAddAccountScript = `$ErrorActionPreference = "Continue"

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
`

// EnsureAddAccountScripts verifies and generates add_google_account.sh and .ps1 if missing.
func EnsureAddAccountScripts() {
	sDir := getScriptsDir()
	shPath := filepath.Join(sDir, "add_google_account.sh")
	if _, err := os.Stat(shPath); os.IsNotExist(err) {
		_ = os.WriteFile(shPath, []byte(shAddAccountScript), 0755)
	} else {
		_ = os.Chmod(shPath, 0755)
	}

	psPath := filepath.Join(sDir, "add_google_account.ps1")
	if _, err := os.Stat(psPath); os.IsNotExist(err) {
		_ = os.WriteFile(psPath, []byte(psAddAccountScript), 0755)
	}
}

// StartInteractiveLoginTerminal opens a terminal window on macOS, Windows, or Linux to execute the login helper.
func StartInteractiveLoginTerminal() error {
	EnsureAddAccountScripts()
	sDir := getScriptsDir()

	switch runtime.GOOS {
	case "darwin":
		shPath := filepath.Join(sDir, "add_google_account.sh")
		cmd := exec.Command("open", "-a", "Terminal", shPath)
		return cmd.Start()

	case "windows":
		psPath := filepath.Join(sDir, "add_google_account.ps1")
		cmd := exec.Command("cmd.exe", "/c", "start", "powershell.exe", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", psPath)
		return cmd.Start()

	case "linux":
		shPath := filepath.Join(sDir, "add_google_account.sh")
		terminals := []struct {
			bin  string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", shPath}},
			{"gnome-terminal", []string{"--", shPath}},
			{"konsole", []string{"-e", shPath}},
			{"xfce4-terminal", []string{"-e", shPath}},
			{"alacritty", []string{"-e", shPath}},
			{"kitty", []string{shPath}},
			{"xterm", []string{"-e", shPath}},
		}
		for _, t := range terminals {
			if p, err := exec.LookPath(t.bin); err == nil && p != "" {
				cmd := exec.Command(t.bin, t.args...)
				return cmd.Start()
			}
		}
		exe, _ := os.Executable()
		return fmt.Errorf("nenhum emulador de terminal gráfico encontrado. Execute no terminal: %s add-account", exe)

	default:
		return fmt.Errorf("sistema operacional não suportado: %s", runtime.GOOS)
	}
}

// RunInteractiveAddAccount handles the CLI mode './antigravity-server add-account'.
func RunInteractiveAddAccount() {
	geminiDir := getGeminiDir()
	profilesDir := getProfilesDir()
	credsPath := filepath.Join(geminiDir, "oauth_creds.json")
	bakPath := filepath.Join(geminiDir, "oauth_creds.json.bak")
	_ = os.MkdirAll(profilesDir, 0755)

	fmt.Println()
	fmt.Println("==========================================================")
	fmt.Println("  🔐 Antigravity - Assistente de Conexão de Nova Conta")
	fmt.Println("==========================================================")
	fmt.Println()

	if _, err := os.Stat(credsPath); err == nil {
		if curEmail, _, _, err := extractInfoFromCreds(credsPath); err == nil && curEmail != "" {
			curDir := filepath.Join(profilesDir, curEmail)
			_ = os.MkdirAll(curDir, 0755)
			_ = copyFile(credsPath, filepath.Join(curDir, "oauth_creds.json"))
			fmt.Printf("💾 Backup da conta atual realizado: %s\n", curEmail)
		}
		_ = os.Rename(credsPath, bakPath)
	}

	fmt.Println("👉 Uma janela do navegador será aberta pelo Antigravity CLI oficial.")
	fmt.Println("👉 Faça login com a NOVA CONTA GOOGLE que deseja adicionar.")
	fmt.Println("👉 Quando concluir no navegador, digite 'exit' no terminal do Antigravity.")
	fmt.Println("----------------------------------------------------------")

	agyBin := config.AppConfig.AgyBin
	if agyBin == "" || agyBin == "agy" {
		if p, err := exec.LookPath("agy"); err == nil && p != "" {
			agyBin = p
		} else {
			home, _ := os.UserHomeDir()
			agyBin = filepath.Join(home, ".local", "bin", "agy")
		}
	}

	cmd := exec.Command(agyBin)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	if _, err := os.Stat(credsPath); err == nil {
		newEmail, name, pic, err := extractInfoFromCreds(credsPath)
		if err == nil && newEmail != "" {
			newDir := filepath.Join(profilesDir, newEmail)
			_ = os.MkdirAll(newDir, 0755)
			_ = copyFile(credsPath, filepath.Join(newDir, "oauth_creds.json"))

			info := AccountProfile{
				Email:    newEmail,
				Name:     name,
				Picture:  pic,
				IsActive: true,
				LastUsed: time.Now().UnixMilli(),
			}
			b, _ := json.MarshalIndent(info, "", "  ")
			_ = os.WriteFile(filepath.Join(newDir, "info.json"), b, 0644)
			_ = os.Remove(bakPath)

			fmt.Printf("\n🎉 Sucesso! Nova conta vinculada: %s\n", newEmail)
			fmt.Println("📱 Ela já está pronta no seu app mobile para alternar com 1 toque!")

			port := config.AppConfig.Port
			if port == "" {
				port = "4040"
			}
			switchBody := fmt.Sprintf(`{"email":"%s"}`, newEmail)
			resp, sErr := http.Post("http://127.0.0.1:"+port+"/api/accounts/switch", "application/json", strings.NewReader(switchBody))
			if sErr == nil && resp != nil {
				_ = resp.Body.Close()
			}

			fmt.Println("\nPressione Enter para fechar...")
			buf := make([]byte, 1)
			_, _ = os.Stdin.Read(buf)
			return
		}
	}

	if _, err := os.Stat(bakPath); err == nil {
		fmt.Println("⚠️ Login cancelado. Restaurando conta anterior...")
		_ = os.Rename(bakPath, credsPath)
	}
	fmt.Println("❌ Nenhuma nova conta foi configurada.")
	fmt.Println("\nPressione Enter para fechar...")
	buf := make([]byte, 1)
	_, _ = os.Stdin.Read(buf)
}
