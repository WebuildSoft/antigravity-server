package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"antigravity-bridge/auth"
	"antigravity-bridge/config"
	"antigravity-bridge/services"
)

// PanicRecoveryMiddleware intercepts unhandled panics in HTTP handlers, records stack traces,
// dispatches an alert to the Discord webhook, and returns a safe 500 JSON response.
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stackBuf := make([]byte, 4096)
				n := runtime.Stack(stackBuf, false)
				stackTrace := string(stackBuf[:n])
				errStr := fmt.Sprintf("%v", rec)

				fmt.Printf("[HTTP Panic Recovered] %s %s: %s\n%s\n", r.Method, r.URL.Path, errStr, stackTrace)

				services.ReportCrash("HTTP_PANIC", errStr, stackTrace, map[string]interface{}{
					"method": r.Method,
					"path":   r.URL.Path,
					"remote": r.RemoteAddr,
				})

				respondJSON(w, http.StatusInternalServerError, map[string]interface{}{
					"error": "Erro interno no servidor interceptado e reportado com segurança.",
					"code":  "INTERNAL_SERVER_ERROR",
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func RegisterHTTPRoutes(mux *http.ServeMux) {
	// Static and Landing Pages
	mux.HandleFunc("/", handleRootAndStatic)
	mux.HandleFunc("/baixar", handleLandingPage)
	mux.HandleFunc("/install", handleLandingPage)
	mux.HandleFunc("/download", handleLandingPage)
	mux.HandleFunc("/app.apk", handleAPKDownload)
	mux.HandleFunc("/AntigravityMobile", handleAPKDownload)
	mux.HandleFunc("/flutter_service_worker.js", handleServiceWorker)

	// System & Status
	mux.HandleFunc("/api/status", handleStatus)
	mux.HandleFunc("/api/version", handleVersion)
	mux.HandleFunc("/api/version/notify", handleVersionNotify)
	mux.HandleFunc("/api/quota", handleQuota)

	// Accounts & OAuth2
	mux.HandleFunc("/api/accounts", handleAccounts)
	mux.HandleFunc("/api/accounts/switch", handleAccountSwitch)
	mux.HandleFunc("/api/accounts/import", handleAccountImport)
	mux.HandleFunc("/api/accounts/auth-url", handleAccountAuthURL)
	mux.HandleFunc("/api/accounts/exchange-code", handleAccountExchangeCode)
	mux.HandleFunc("/api/accounts/start-login", handleAccountStartLogin)
	mux.HandleFunc("/oauth2callback", handleOAuth2Callback)
	mux.HandleFunc("/login-google", handleGoogleLoginRedirect)

	// Conversations
	mux.HandleFunc("/api/conversations", handleConversations)
	mux.HandleFunc("/api/conversations/", handleSingleConversation)

	// Workspace
	mux.HandleFunc("/api/workspace/tree", handleWorkspaceTree)
	mux.HandleFunc("/api/workspace/file", handleWorkspaceFile)

	// Upload & Media
	mux.HandleFunc("/api/upload", handleUpload)
	mux.HandleFunc("/api/media", handleMedia)
	mux.HandleFunc("/uploads/", handleUploads)
}

func enableCORS(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin != "" && isAllowedOrigin(r) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else if origin == "" {
		w.Header().Set("Access-Control-Allow-Origin", "*")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Standard Security Headers
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
}

func authenticateRequest(r *http.Request) (*auth.FirebaseUser, error) {
	origin := r.Header.Get("Origin")
	if origin != "" && !isAllowedOrigin(r) {
		return nil, errors.New("origem não autorizada")
	}

	authHeader := r.Header.Get("Authorization")
	token := strings.TrimPrefix(authHeader, "Bearer ")
	if token == "" {
		token = r.URL.Query().Get("token")
	}

	isRemote := !isLoopbackAddr(r.RemoteAddr, r.Header)

	if token != "" {
		user, err := auth.VerifyFirebaseToken(token)
		if err != nil || user == nil {
			return nil, fmt.Errorf("token inválido ou expirado: %v", err)
		}
		if !config.AppConfig.IsEmailAllowed(user.Email) {
			return nil, fmt.Errorf("usuário não autorizado: %s", user.Email)
		}
		return user, nil
	}

	if isRemote {
		return nil, errors.New("token de autenticação obrigatório para acesso remoto")
	}

	return &auth.FirebaseUser{
		UID:   "local-dev",
		Email: "developer@localhost",
	}, nil
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	respondJSON(w, http.StatusOK, services.GetServerStatus(services.Manager.ActiveSessionsCount()))
}

func handleVersion(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	respondJSON(w, http.StatusOK, services.LoadVersionData())
}

func handleVersionNotify(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	vData := services.LoadVersionData()
	fmt.Printf("[WS] 📢 Broadcasting new version: v%s (Build %d)\n", vData.Version, vData.Build)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":    true,
		"appVersion": vData,
	})
}

func handleQuota(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	force := r.URL.Query().Get("refresh") == "true"
	data, err := services.GetQuotaData(force)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, data)
}
