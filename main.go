package main

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
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

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorGray   = "\033[90m"
	colorWhite  = "\033[97m"
)

func boxPad(text string, width int) string {
	runes := []rune(text)
	if len(runes) > width {
		return string(runes[:width-3]) + "..."
	}
	return text + strings.Repeat(" ", width-len(runes))
}

func printQRCode(targetURL string) {
	qr, err := qrcode.New(targetURL, qrcode.Medium)
	if err != nil {
		return
	}
	fmt.Println()
	fmt.Printf("  %s╭─────────────────────────────────────────────────────────────╮%s\n", colorPurple, colorReset)
	fmt.Printf("  %s│%s  %s  %s│%s\n", colorPurple, colorReset, boxPad("PAREAMENTO RÁPIDO COM O ANTIGRAVITY MOBILE", 57), colorPurple, colorReset)
	fmt.Printf("  %s│%s  %s  %s│%s\n", colorPurple, colorReset, boxPad("Aponte a câmera do aplicativo para o código abaixo:", 57), colorPurple, colorReset)
	linkText := "Link: " + targetURL
	fmt.Printf("  %s│%s  %s%s%s  %s│%s\n", colorPurple, colorReset, colorCyan, boxPad(linkText, 57), colorReset, colorPurple, colorReset)
	fmt.Printf("  %s╰─────────────────────────────────────────────────────────────╯%s\n", colorPurple, colorReset)
	fmt.Println()
	fmt.Print(qr.ToSmallString(false))
	fmt.Printf("  %s[INFO]%s %s Aguardando conexões do aplicativo móvel...\n\n", colorGreen, colorReset, time.Now().Format("15:04:05"))
}

func printServerDashboard(port string, ips []services.NetworkAddress, publicURL string, ver string, build int) {
	fmt.Println()
	title := fmt.Sprintf("ANTIGRAVITY SERVER • v%s (Build #%d)", ver, build)
	sub := "WebuildSoft Engine • Alta Performance & Latência Zero"

	fmt.Printf("%s╭─────────────────────────────────────────────────────────────╮%s\n", colorCyan, colorReset)
	fmt.Printf("%s│%s  %s  %s│%s\n", colorCyan, colorReset, boxPad(title, 57), colorCyan, colorReset)
	fmt.Printf("%s│%s  %s  %s│%s\n", colorCyan, colorReset, colorDim+boxPad(sub, 57)+colorReset, colorCyan, colorReset)
	fmt.Printf("%s╰─────────────────────────────────────────────────────────────╯%s\n", colorCyan, colorReset)

	fmt.Println()
	fmt.Printf("  %s●%s %sSistema:%s    %s (%s/%s)\n", colorGreen, colorReset, colorBold, colorReset, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  %s●%s %sPorta:%s      %s (TCP)\n", colorGreen, colorReset, colorBold, colorReset, port)
	fmt.Printf("  %s●%s %sWorkspace:%s  %s\n", colorGreen, colorReset, colorBold, colorReset, config.AppConfig.DefaultWorkspace)

	// Status do CLI agy
	agyStatus := fmt.Sprintf("%sConectado (%s)%s", colorGreen, config.AppConfig.AgyBin, colorReset)
	if config.AppConfig.AgyBin == "" || config.AppConfig.AgyBin == "agy" {
		agyStatus = fmt.Sprintf("%sDetectando no PATH%s", colorYellow, colorReset)
	}
	fmt.Printf("  %s●%s %sCLI Agente:%s %s\n", colorGreen, colorReset, colorBold, colorReset, agyStatus)

	// Status de Diagnóstico / Telemetria
	if config.AppConfig.TelemetryEnabled {
		fmt.Printf("  %s●%s %sDiagnóstico:%s %sAtivo (Relatórios Anônimos WebuildSoft)%s\n", colorGreen, colorReset, colorBold, colorReset, colorGreen, colorReset)
	} else {
		fmt.Printf("  %s●%s %sDiagnóstico:%s %sDesativado (Privacidade Total)%s\n", colorGray, colorReset, colorBold, colorReset, colorGray, colorReset)
	}

	fmt.Println()
	fmt.Printf("  %s%sENDPOINTS DE CONEXÃO:%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("    Local:   %shttp://localhost:%s%s\n", colorWhite, port, colorReset)
	for _, ip := range ips {
		fmt.Printf("    Rede:    %shttp://%s:%s%s %s(ws://%s:%s/ws)%s\n",
			colorWhite, ip.Address, port, colorReset, colorGray, ip.Address, port, colorReset)
	}
	if publicURL != "" {
		fmt.Printf("    Túnel:   %s%s%s %s(TLS Criptografado)%s\n",
			colorGreen, publicURL, colorReset, colorDim, colorReset)
	}

	targetURL := publicURL
	if targetURL == "" && len(ips) > 0 {
		targetURL = fmt.Sprintf("http://%s:%s", ips[0].Address, port)
	} else if targetURL == "" {
		targetURL = fmt.Sprintf("http://localhost:%s", port)
	}
	printQRCode(targetURL)
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
			fmt.Printf("  %s[HTTP]%s %s %-6s %s%s%s %s(%s)%s\n",
				colorGreen, colorReset, time.Now().Format("15:04:05"), r.Method, colorCyan, r.URL.Path, colorReset, colorGray, clientIP, colorReset)
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

	vInfo := services.LoadVersionData()
	ver := vInfo.Version
	if ver == "" {
		ver = "1.4.0"
	}
	build := vInfo.Build
	if build == 0 {
		build = 25
	}

	cleanPublicURL := strings.TrimSpace(config.AppConfig.PublicURL)
	if cleanPublicURL == `""` {
		cleanPublicURL = ""
	}

	printServerDashboard(port, ips, cleanPublicURL, ver, build)

	server := &http.Server{
		Handler:           gateway.PanicRecoveryMiddleware(handler),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Printf("Server error: %v\n", err)
	}
}
