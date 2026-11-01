package main

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
	"antigravity-bridge/gateway"
	"antigravity-bridge/services"
	"github.com/gorilla/websocket"
	"github.com/skip2/go-qrcode"
)

func printQRCode(targetURL string) {
	qr, err := qrcode.New(targetURL, qrcode.Medium)
	if err != nil {
		return
	}
	fmt.Println("\n============================================================")
	fmt.Printf("  📱 CONEXÃO RÁPIDA - ANTIGRAVITY MOBILE\n")
	fmt.Printf("  Link: %s\n", targetURL)
	fmt.Println("  (Abra o app no celular e escaneie o código abaixo)")
	fmt.Println("============================================================")
	fmt.Print(qr.ToSmallString(false))
	fmt.Println("============================================================\n")
}

func main() {
	config.InitConfig()

	// Initialize SQLite Database
	if err := db.InitDB(); err != nil {
		fmt.Printf("[DB] ⚠️ Warning: Failed to initialize SQLite database: %v\n", err)
	} else {
		go db.SyncLegacyBrainTranscripts()
	}

	services.InitSessionManager()
	services.InitAccountsManager()
	services.StartQuotaBackgroundWorker()

	mux := http.NewServeMux()

	// Register all HTTP REST and static routes
	gateway.RegisterHTTPRoutes(mux)

	// Explicit /ws endpoint
	mux.HandleFunc("/ws", gateway.HandleWebSocket)

	// Custom handler for root "/" to seamlessly upgrade to WebSocket if client requests it
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Log incoming HTTP requests (ignoring internal heavy asset requests)
		if !strings.HasPrefix(r.URL.Path, "/canvaskit/") && !strings.HasPrefix(r.URL.Path, "/assets/") {
			clientIP := r.RemoteAddr
			if cfIP := r.Header.Get("CF-Connecting-IP"); cfIP != "" {
				clientIP = cfIP + " (Cloudflare)"
			} else if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
				clientIP = strings.Split(xff, ",")[0]
			}
			fmt.Printf("🌐 [HTTP] %s %s from %s\n", r.Method, r.URL.Path, clientIP)
		}
		if (r.URL.Path == "/" || r.URL.Path == "/ws") && websocket.IsWebSocketUpgrade(r) {
			gateway.HandleWebSocket(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})

	startPort, _ := strconv.Atoi(config.AppConfig.Port)
	if startPort <= 0 {
		startPort = 4040
	}

	var listener net.Listener
	var finalPort int
	for p := startPort; p < startPort+50; p++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			listener = l
			finalPort = p
			break
		}
		if p == startPort {
			fmt.Printf("[Server] ⚠️ Porta %d ocupada. Procurando próxima porta disponível...\n", p)
		}
	}

	if listener == nil {
		fmt.Printf("❌ Erro fatal: Nenhuma porta disponível a partir de %d\n", startPort)
		return
	}
	defer listener.Close()

	port := strconv.Itoa(finalPort)
	config.AppConfig.Port = port
	if finalPort != startPort {
		fmt.Printf("[Server] ✓ Porta livre encontrada e configurada automaticamente: %s\n", port)
	}

	ips := services.GetNetworkAddresses()

	fmt.Printf("\n🚀 Antigravity Mobile Bridge listening on port %s (Go v2.0)\n", port)
	fmt.Printf("Local endpoints:\n")
	fmt.Printf("  - Local:   http://localhost:%s\n", port)
	for _, ip := range ips {
		fmt.Printf("  - Network: http://%s:%s (ws://%s:%s/ws)\n", ip.Address, port, ip.Address, port)
	}

	// Print QR Code
	cleanPublicURL := strings.TrimSpace(config.AppConfig.PublicURL)
	if cleanPublicURL != "" && cleanPublicURL != `""` {
		printQRCode(cleanPublicURL)
	} else if len(ips) > 0 {
		printQRCode(fmt.Sprintf("http://%s:%s", ips[0].Address, port))
	} else {
		printQRCode(fmt.Sprintf("http://localhost:%s", port))
	}

	if config.AppConfig.TelemetryEnabled && config.AppConfig.ErrorWebhookURL != "" {
		fmt.Println("[Telemetry] 🛡️ Monitoramento de erros ativo -> Discord Webhook configurado")
	} else if !config.AppConfig.TelemetryEnabled {
		fmt.Println("[Telemetry] 🛑 Telemetria desativada pelo usuário (TELEMETRY_ENABLED=false)")
	} else {
		fmt.Println("[Telemetry] ℹ️ Telemetria Discord pendente (Defina ERROR_WEBHOOK_URL no .env para receber alertas)")
	}

	server := &http.Server{
		Handler:           gateway.PanicRecoveryMiddleware(handler),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
