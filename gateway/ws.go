package gateway

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"time"

	"antigravity-bridge/auth"
	"antigravity-bridge/config"
	"antigravity-bridge/services"
	"github.com/gorilla/websocket"
)

func isLoopbackAddr(remoteAddr string, header http.Header) bool {
	// If behind a proxy or Cloudflare tunnel, it is remote!
	if header.Get("CF-Connecting-IP") != "" || header.Get("X-Forwarded-For") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}
	return ip.IsLoopback()
}

func isAllowedOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Native mobile apps and non-browser WebSocket clients do not send Origin header
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return true
	}
	if strings.HasSuffix(host, ".trycloudflare.com") {
		return true
	}
	if config.AppConfig.PublicURL != "" {
		if pubU, err := url.Parse(config.AppConfig.PublicURL); err == nil {
			if strings.EqualFold(pubU.Hostname(), host) {
				return true
			}
		}
	}
	reqHost, _, _ := net.SplitHostPort(r.Host)
	if reqHost == "" {
		reqHost = r.Host
	}
	return strings.EqualFold(reqHost, host)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024 * 64,
	WriteBufferSize: 1024 * 64,
	CheckOrigin: func(r *http.Request) bool {
		return isAllowedOrigin(r)
	},
}

type ClientConn struct {
	sync.Mutex
	ws       *websocket.Conn
	authUser *auth.FirebaseUser
	session  *services.Session
	isAlive  bool
}

func (c *ClientConn) Send(payload interface{}) error {
	c.Lock()
	defer c.Unlock()
	return c.ws.WriteJSON(payload)
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rec := recover(); rec != nil {
			stackBuf := make([]byte, 4096)
			n := runtime.Stack(stackBuf, false)
			stackTrace := string(stackBuf[:n])
			errStr := fmt.Sprintf("%v", rec)

			fmt.Printf("[WS Connection Panic Recovered] %s: %s\n%s\n", r.RemoteAddr, errStr, stackTrace)

			services.ReportCrash("WS_CONNECTION_PANIC", errStr, stackTrace, map[string]interface{}{
				"remoteAddr": r.RemoteAddr,
			})
		}
	}()

	clientIP := r.RemoteAddr
	if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
		clientIP = cfIP + " (Cloudflare)"
	} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		clientIP = strings.Split(xff, ",")[0]
	}

	fmt.Printf("📱 [WS] Nova solicitacao WebSocket de %s (Host: %s, Origin: %s)\n", clientIP, r.Host, r.Header.Get("Origin"))

	if !websocket.IsWebSocketUpgrade(r) {
		fmt.Printf("⚠️ [WS] Falha: requisicao nao contem headers de WebSocket Upgrade\n")
		http.Error(w, "WebSocket connection expected (Upgrade: websocket)", http.StatusBadRequest)
		return
	}
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		fmt.Printf("❌ [WS Handshake Error] %v (Origin: %s, Host: %s)\n", err, r.Header.Get("Origin"), r.Host)
		return
	}
	ws.SetReadLimit(100 * 1024 * 1024) // 100 MB para permitir mensagens com múltiplas fotos em alta resolução

	token := r.URL.Query().Get("token")
	isRemote := !isLoopbackAddr(r.RemoteAddr, r.Header)

	client := &ClientConn{
		ws:      ws,
		isAlive: true,
	}

	if token != "" {
		user, err := auth.VerifyFirebaseToken(token)
		if err != nil || user == nil {
			fmt.Printf("[Firebase Auth] ⛔ Token inválido de %s: %v\n", clientIP, err)
			_ = client.Send(map[string]interface{}{
				"type":    "auth_error",
				"code":    "invalid_token",
				"message": "Token de autenticação inválido ou expirado.",
			})
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4001, "Invalid Token"),
				time.Now().Add(time.Second))
			_ = ws.Close()
			return
		}

		if !config.AppConfig.IsEmailAllowed(user.Email) {
			fmt.Printf("\n============================================================\n")
			fmt.Printf("⛔ [ACESSO BLOQUEADO] Tentativa com e-mail não autorizado: %s (%s)\n", user.Email, clientIP)
			fmt.Printf("💡 Para autorizar este e-mail, adicione no seu .env:\n")
			fmt.Printf("   ALLOWED_EMAILS=\"%s\"\n", user.Email)
			fmt.Printf("============================================================\n\n")
			_ = client.Send(map[string]interface{}{
				"type":    "auth_error",
				"code":    "unauthorized_user",
				"message": "Acesso restrito. Este usuário não tem autorização para utilizar este sistema.",
			})
			_ = ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4003, "Unauthorized User"),
				time.Now().Add(time.Second))
			_ = ws.Close()
			return
		}
		client.authUser = user
		fmt.Printf("\n📱 [APP CONECTADO] Usuário autenticado com sucesso: %s (%s)\n\n", user.Email, clientIP)
	} else if isRemote {
		// Camada 2: Conexão remota SEM token é terminantemente bloqueada
		fmt.Printf("[WS Auth] ⛔ Conexão REMOTA rejeitada (Token Ausente): %s\n", clientIP)
		_ = client.Send(map[string]interface{}{
			"type":    "auth_error",
			"code":    "token_required",
			"message": "Acesso remoto restrito. O token de autenticação Firebase é obrigatório.",
		})
		_ = ws.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(4001, "Token Required"),
			time.Now().Add(time.Second))
		_ = ws.Close()
		return
	}

	authDisplay := "pendente/local"
	userEmail := ""
	if client.authUser != nil {
		authDisplay = client.authUser.Email
		userEmail = client.authUser.Email
	}
	fmt.Printf("[WS] Client connected from %s (Auth: %s)\n", clientIP, authDisplay)

	userConvs := services.ListConversationsForUser(userEmail)

	// Attach to latest session for this user initially
	latestID := ""
	if len(userConvs) > 0 {
		latestID = userConvs[0].ID
	}
	if latestID == "" {
		latestID = "default"
	}

	session := services.Manager.GetSession(latestID)
	session.Lock()
	session.Clients[ws] = true
	session.Unlock()
	client.session = session

	// Send welcome connection packet
	// Sanitize output for remote clients to prevent leakage of host paths, accounts or private LAN IPs
	var clientIPs []services.NetworkAddress
	var clientAccounts []services.AccountProfile
	clientWorkspace := "~"

	if !isRemote {
		clientIPs = services.GetNetworkAddresses()
		clientAccounts, _ = services.ListAccounts()
		clientWorkspace = config.AppConfig.DefaultWorkspace
	} else {
		clientIPs = []services.NetworkAddress{}
		if userEmail != "" {
			allAccs, _ := services.ListAccounts()
			for _, a := range allAccs {
				if strings.EqualFold(a.Email, userEmail) {
					clientAccounts = append(clientAccounts, a)
				}
			}
		}
	}
	if clientAccounts == nil {
		clientAccounts = []services.AccountProfile{}
	}

	_ = client.Send(map[string]interface{}{
		"type":                 "connected",
		"message":              "Connected to Antigravity Mobile Bridge (Go)",
		"ips":                  clientIPs,
		"conversations":        userConvs,
		"latestConversationId": latestID,
		"appVersion":           services.LoadVersionData(),
		"accounts":             clientAccounts,
		"workspace":            clientWorkspace,
		"telemetryEnabled":     config.AppConfig.TelemetryEnabled,
	})

	// Heartbeat pong handler
	ws.SetPongHandler(func(string) error {
		client.Lock()
		client.isAlive = true
		client.Unlock()
		return nil
	})

	// Background reader loop
	for {
		_, rawMsg, err := ws.ReadMessage()
		if err != nil {
			fmt.Printf("[WS] Client disconnected from %s (background tasks continue unaffected)\n", clientIP)
			if client.session != nil {
				client.session.Lock()
				delete(client.session.Clients, ws)
				client.session.Unlock()
			}
			break
		}

		var data map[string]interface{}
		if err := json.Unmarshal(rawMsg, &data); err != nil {
			continue
		}

		action, _ := data["action"].(string)
		client.dispatchAction(action, data, isRemote)
	}
}
