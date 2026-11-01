package tests

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
	"antigravity-bridge/gateway"
	"antigravity-bridge/services"
	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

func setupTestEnvironment(t *testing.T) (*httptest.Server, func()) {
	tmpDir, err := os.MkdirTemp("", "antigravity_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	config.AppConfig = config.Config{
		Port:             "0",
		AllowedEmails:    []string{"dev@teste.com", "equipe@empresa.com"},
		DefaultWorkspace: tmpDir,
		UploadsDir:       filepath.Join(tmpDir, "uploads"),
		BrainDir:         filepath.Join(tmpDir, "brain"),
		TelemetryEnabled: true,
	}
	_ = os.MkdirAll(config.AppConfig.UploadsDir, 0755)
	_ = os.MkdirAll(filepath.Join(tmpDir, "data"), 0755)

	dbPath := filepath.Join(tmpDir, "data", "antigravity.db")
	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db.DB, err = sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("Failed to open test SQLite: %v", err)
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		thinking TEXT,
		image_url TEXT,
		image_urls TEXT,
		created_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS tool_calls (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		name TEXT NOT NULL,
		summary TEXT NOT NULL,
		args TEXT,
		output TEXT,
		status TEXT NOT NULL
	);
	`
	if _, err := db.DB.Exec(schema); err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	services.InitSessionManager()

	mux := http.NewServeMux()
	gateway.RegisterHTTPRoutes(mux)
	mux.HandleFunc("/ws", gateway.HandleWebSocket)

	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		if db.DB != nil {
			_ = db.DB.Close()
		}
		_ = os.RemoveAll(tmpDir)
	}

	return server, cleanup
}

// Test 1: Conexão remota (via Cloudflare header) sem token DEVE ser bloqueada!
func TestRemoteConnectionWithoutTokenRejected(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header := http.Header{}
	header.Set("CF-Connecting-IP", "198.51.100.25") // Simula Cloudflare Tunnel

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, header)

	if err == nil && conn != nil {
		// Even if upgraded, the server must immediately send auth_error and close
		var msg map[string]interface{}
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		readErr := conn.ReadJSON(&msg)
		_ = conn.Close()

		if readErr == nil && msg["code"] == "token_required" {
			t.Logf("✓ Conexão remota sem token rejeitada com sucesso com código: %v", msg["code"])
			return
		}
		t.Fatalf("FALHA DE SEGURANÇA: Conexão remota sem token foi aceita sem bloqueio!")
	}

	if resp != nil && (resp.StatusCode == 4001 || resp.StatusCode == 403 || resp.StatusCode == 401) {
		t.Logf("✓ Conexão remota rejeitada no handshake HTTP: status %d", resp.StatusCode)
		return
	}

	t.Logf("✓ Conexão remota bloqueada pelo dialer: %v", err)
}

// Test 2: Token forjado ou inválido DEVE ser rejeitado!
func TestInvalidTokenRejected(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws?token=token_falso_forjado_xyz123"

	header := http.Header{}
	header.Set("CF-Connecting-IP", "198.51.100.25")

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, header)

	if err == nil && conn != nil {
		var msg map[string]interface{}
		_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		readErr := conn.ReadJSON(&msg)
		_ = conn.Close()

		if readErr == nil && msg["code"] == "invalid_token" {
			t.Logf("✓ Token inválido rejeitado com sucesso com código: %v", msg["code"])
			return
		}
		t.Fatalf("FALHA DE SEGURANÇA: Token forjado foi aceito pelo servidor!")
	}
	t.Logf("✓ Token inválido bloqueado com sucesso.")
}

// Test 3: Conexão Localhost (Loopback do Desenvolvedor) é aceita
func TestLocalhostConnectionAccepted(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// No CF-Connecting-IP header -> loopback direto da máquina
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Falha ao conectar no localhost: %v", err)
	}
	defer conn.Close()

	var msg map[string]interface{}
	_ = conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("Falha ao ler pacote de boas-vindas: %v", err)
	}

	if msg["type"] != "connected" {
		t.Fatalf("Tipo inesperado de mensagem: got %v, want connected", msg["type"])
	}
	t.Logf("✓ Conexão local em localhost autorizada com sucesso.")
}

// Test 4: Isolamento de Conversas por Usuário (Camada 4)
func TestConversationIsolation(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Inserir conversa do Usuario A
	now := time.Now().UTC()
	err := db.UpsertConversation("conv_joao_1", "joao@teste.com", "Chat Secreto do João", now)
	if err != nil {
		t.Fatalf("Failed to insert João's conversation: %v", err)
	}

	// Inserir conversa do Usuario B
	err = db.UpsertConversation("conv_maria_1", "maria@teste.com", "Chat Secreto da Maria", now)
	if err != nil {
		t.Fatalf("Failed to insert Maria's conversation: %v", err)
	}

	// Buscar conversas do João
	joaoConvs, err := db.ListConversationsFromDB("joao@teste.com")
	if err != nil {
		t.Fatalf("Erro ao buscar conversas do João: %v", err)
	}

	if len(joaoConvs) != 1 || joaoConvs[0].ID != "conv_joao_1" {
		t.Fatalf("FALHA DE ISOLAMENTO: João recebeu conversas erradas: %v", joaoConvs)
	}

	// Buscar conversas da Maria
	mariaConvs, err := db.ListConversationsFromDB("maria@teste.com")
	if err != nil {
		t.Fatalf("Erro ao buscar conversas da Maria: %v", err)
	}

	if len(mariaConvs) != 1 || mariaConvs[0].ID != "conv_maria_1" {
		t.Fatalf("FALHA DE ISOLAMENTO: Maria recebeu conversas erradas: %v", mariaConvs)
	}

	// Verificar checagem de posse
	ownerJoao, _ := db.GetConversationOwner("conv_joao_1")
	if ownerJoao != "joao@teste.com" {
		t.Fatalf("Posse incorreta: got %v, want joao@teste.com", ownerJoao)
	}

	t.Logf("✓ Isolamento de dados entre João e Maria 100%% comprovado!")
}

// Test 5: Tentativa de Conexão com Origem Maliciosa (CSWSH) DEVE ser Bloqueada!
func TestDisallowedOriginRejected(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	header := http.Header{}
	header.Set("Origin", "https://site-malicioso-hacker.com")

	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.Dial(wsURL, header)
	if conn != nil {
		_ = conn.Close()
	}

	if err == nil {
		t.Fatalf("FALHA DE SEGURANÇA: Origem maliciosa externa foi aceita pelo WebSocket!")
	}

	if resp != nil && (resp.StatusCode == http.StatusForbidden || resp.StatusCode == 4003 || resp.StatusCode == 4001) {
		t.Logf("✓ Origem maliciosa bloqueada com status HTTP %d", resp.StatusCode)
		return
	}

	t.Logf("✓ Origem maliciosa rejeitada com sucesso: %v", err)
}

// Test 6: Tentativa de Path Traversal em /api/media DEVE ser Bloqueada!
func TestMediaArbitraryFileReadBlocked(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Tentativa de ler arquivo de fora do diretório permitido (ex: /etc/passwd)
	resp, err := http.Get(server.URL + "/api/media?path=/etc/passwd")
	if err != nil {
		t.Fatalf("Erro ao fazer requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
		t.Fatalf("FALHA DE SEGURANÇA: Leitura de arquivo arbitrário retornou status %d (esperava 404/403)!", resp.StatusCode)
	}

	t.Logf("✓ Tentativa de Path Traversal em /api/media bloqueada com status %d", resp.StatusCode)
}

// Test 7: Acesso Remoto a /api/conversations sem Token DEVE retornar 401!
func TestRemoteConversationsWithoutTokenRejected(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/conversations", nil)
	if err != nil {
		t.Fatalf("Erro ao criar requisição: %v", err)
	}
	req.Header.Set("CF-Connecting-IP", "198.51.100.50") // Simula acesso remoto

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Erro ao executar requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("FALHA DE SEGURANÇA: Requisição remota sem token recebeu status %d (esperava 401)!", resp.StatusCode)
	}

	t.Logf("✓ Requisição remota a /api/conversations sem token bloqueada com 401")
}

// Test 8: Acesso Remoto a /api/workspace/file sem Token DEVE retornar 401!
func TestRemoteWorkspaceWithoutTokenRejected(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/workspace/file?path=main.go", nil)
	if err != nil {
		t.Fatalf("Erro ao criar requisição: %v", err)
	}
	req.Header.Set("CF-Connecting-IP", "198.51.100.50") // Simula acesso remoto

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Erro ao executar requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("FALHA DE SEGURANÇA: Requisição remota a workspace sem token recebeu status %d (esperava 401)!", resp.StatusCode)
	}

	t.Logf("✓ Requisição remota a /api/workspace sem token bloqueada com 401")
}

// Test 9: Validação de Métodos HTTP e Cabeçalhos de Segurança
func TestHTTPMethodsAndSecurityHeaders(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// GET em /api/status deve retornar 200 e conter cabeçalhos de segurança
	resp, err := http.Get(server.URL + "/api/status")
	if err != nil {
		t.Fatalf("Erro ao consultar /api/status: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status inesperado em GET /api/status: %d", resp.StatusCode)
	}
	if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("Cabeçalho X-Content-Type-Options ausente ou incorreto: %s", resp.Header.Get("X-Content-Type-Options"))
	}
	if resp.Header.Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Errorf("Cabeçalho X-Frame-Options ausente ou incorreto: %s", resp.Header.Get("X-Frame-Options"))
	}

	// POST em /api/status deve retornar 405 Method Not Allowed
	respPost, err := http.Post(server.URL+"/api/status", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Erro ao enviar POST em /api/status: %v", err)
	}
	respPost.Body.Close()

	if respPost.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("FALHA: POST em /api/status deveria retornar 405, retornou %d", respPost.StatusCode)
	}

	t.Logf("✓ Cabeçalhos de segurança e restrição de método HTTP validados com sucesso")
}

// Test 10: Proteção contra DoS via Limite de Payload (MaxBytesReader)
func TestRequestBodySizeLimit(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Cria payload maior que 64KB (ex: 100KB)
	largePayload := `{"email":"` + strings.Repeat("a", 100*1024) + `"}`
	resp, err := http.Post(server.URL+"/api/accounts/switch", "application/json", strings.NewReader(largePayload))
	if err != nil {
		t.Fatalf("Erro ao enviar payload grande: %v", err)
	}
	resp.Body.Close()

	// Deve ser rejeitado com 400 Bad Request por falha no decode devido a MaxBytesReader
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("FALHA: Payload excessivo (>64KB) deveria retornar 400 Bad Request, retornou %d", resp.StatusCode)
	}

	t.Logf("✓ Limite de tamanho de requisição (MaxBytesReader) bloqueou payload excessivo com sucesso")
}

// Test 11: Requisição remota a /api/version/notify sem autenticação deve retornar 401
func TestRemoteVersionNotifyRequiresAuth(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/version/notify", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("Erro ao criar requisição: %v", err)
	}
	req.Header.Set("CF-Connecting-IP", "198.51.100.50") // Simula acesso remoto

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Erro ao executar requisição: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("FALHA: /api/version/notify remoto sem token deveria retornar 401, retornou %d", resp.StatusCode)
	}

	t.Logf("✓ /api/version/notify remoto bloqueado com 401 para requisições não autenticadas")
}
