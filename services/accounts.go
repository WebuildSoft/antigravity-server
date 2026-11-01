package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
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

	// Invalidate quota cache and refresh quota asynchronously under the new account
	go func() {
		time.Sleep(200 * time.Millisecond)
		InvalidateAndRefreshQuota()
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
