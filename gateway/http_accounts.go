package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"

	"antigravity-bridge/config"
	"antigravity-bridge/services"
)

func handleAccounts(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method == http.MethodDelete {
		email := r.URL.Query().Get("email")
		if err := services.RemoveAccount(email); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"success": true})
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var accounts []services.AccountProfile
	isRemote := !isLoopbackAddr(r.RemoteAddr, r.Header)
	user, _ := authenticateRequest(r)

	if !isRemote {
		accounts, _ = services.ListAccounts()
	} else if user != nil && user.Email != "" {
		accounts, _ = services.ListAccounts()
	}
	if accounts == nil {
		accounts = []services.AccountProfile{}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"accounts": accounts,
	})
}

func handleAccountSwitch(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "JSON inválido"})
		return
	}
	profile, err := services.SwitchAccount(req.Email)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	accounts, _ := services.ListAccounts()
	services.Manager.BroadcastAll(map[string]interface{}{
		"type":          "account_switched",
		"activeAccount": profile,
		"accounts":      accounts,
	})
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"activeAccount": profile,
		"accounts":      accounts,
	})
}

func handleAccountAuthURL(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"authUrl": services.GetGoogleAuthURL(),
	})
}

func handleAccountExchangeCode(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Código de autorização inválido ou vazio"})
		return
	}

	profile, err := services.ExchangeGoogleAuthCode(req.Code)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}

	accounts, _ := services.ListAccounts()
	services.Manager.BroadcastAll(map[string]interface{}{
		"type":          "account_switched",
		"activeAccount": profile,
		"accounts":      accounts,
		"message":       fmt.Sprintf("Conta %s conectada com sucesso!", profile.Email),
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"activeAccount": profile,
		"accounts":      accounts,
	})
}

func handleOAuth2Callback(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Código não encontrado na resposta do Google", http.StatusBadRequest)
		return
	}

	profile, err := services.ExchangeGoogleAuthCode(code)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		fmt.Fprintf(w, `
			<!DOCTYPE html><html><body style="background:#0f172a;color:#ef4444;font-family:sans-serif;text-align:center;padding:50px;">
			<h2>❌ Erro ao autenticar conta</h2><p>%v</p>
			</body></html>
		`, err)
		return
	}

	accounts, _ := services.ListAccounts()
	services.Manager.BroadcastAll(map[string]interface{}{
		"type":          "account_switched",
		"activeAccount": profile,
		"accounts":      accounts,
		"message":       fmt.Sprintf("Conta %s conectada com sucesso!", profile.Email),
	})

	fmt.Fprintf(w, `
		<!DOCTYPE html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"></head>
		<body style="background:#0f172a;color:white;font-family:sans-serif;display:flex;align-items:center;justify-content:center;height:100vh;margin:0;text-align:center;">
		<div style="background:#1e293b;padding:30px;border-radius:16px;max-width:90%%;box-shadow:0 4px 20px rgba(0,0,0,0.5);">
			<h2 style="color:#10b981;margin-bottom:10px;">✅ Conta Conectada com Sucesso!</h2>
			<p style="color:#94a3b8;font-size:15px;">A conta <strong>%s</strong> agora está ativa no seu Antigravity.</p>
			<p style="color:#64748b;font-size:13px;margin-top:20px;">Você já pode voltar para o app mobile.</p>
		</div>
		</body></html>
	`, profile.Email)
}

func handleGoogleLoginRedirect(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	authURL := services.GetGoogleAuthURL()
	http.Redirect(w, r, authURL, http.StatusFound)
}

func handleAccountStartLogin(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	scriptPath := filepath.Join(filepath.Dir(config.AppConfig.VersionFile), "scripts", "add_google_account.sh")
	cmd := exec.Command("open", "-a", "Terminal", scriptPath)
	if err := cmd.Start(); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": fmt.Sprintf("Falha ao abrir terminal: %v", err)})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Terminal aberto no Mac para login Google",
	})
}

func handleAccountImport(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Falha ao ler dados"})
		return
	}
	profile, err := services.ImportAccount(body)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
		return
	}
	accounts, _ := services.ListAccounts()
	services.Manager.BroadcastAll(map[string]interface{}{
		"type":          "account_switched",
		"activeAccount": profile,
		"accounts":      accounts,
	})
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":       true,
		"activeAccount": profile,
		"accounts":      accounts,
	})
}
