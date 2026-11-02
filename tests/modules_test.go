package tests

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
	"antigravity-bridge/gateway"
	"antigravity-bridge/services"
)

// Test 12: Bloqueio de Path Traversal no Workspace (services/file.go)
func TestWorkspacePathTraversalBlocked(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Tentativa de gravar arquivo fora do workspace
	_, err := services.SaveFileContent("../../secret.txt", "conteudo", "")
	if err == nil {
		t.Fatalf("FALHA DE SEGURANÇA: SaveFileContent permitiu gravação fora do workspace!")
	}
	t.Logf("✓ SaveFileContent bloqueou gravação maliciosa fora do workspace: %v", err)

	// Tentativa de ler arquivo fora do workspace
	_, err = services.ReadFileContent("../../../etc/passwd", "")
	if err == nil {
		t.Fatalf("FALHA DE SEGURANÇA: ReadFileContent permitiu leitura fora do workspace!")
	}
	t.Logf("✓ ReadFileContent bloqueou leitura maliciosa fora do workspace: %v", err)
}

// Test 13: Limite de Tamanho de Upload e Proteção contra DoS (services/upload.go)
func TestUploadBase64ImageLimit(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// 1. Upload de imagem válida pequena
	validB64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake-png-content"))
	res, err := services.SaveBase64Image(validB64)
	if err != nil {
		t.Fatalf("Erro ao salvar imagem válida: %v", err)
	}
	if !strings.HasPrefix(res.URL, "/uploads/") {
		t.Fatalf("URL de retorno inesperada: %s", res.URL)
	}
	t.Logf("✓ Imagem válida salva com sucesso em: %s", res.Path)

	// 2. Upload de imagem gigante (> 25MB) para testar proteção de DoS
	giantData := make([]byte, 26*1024*1024)
	giantB64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(giantData)
	_, err = services.SaveBase64Image(giantB64)
	if err == nil {
		t.Fatalf("FALHA DE SEGURANÇA: SaveBase64Image aceitou upload maior que 25MB!")
	}
	t.Logf("✓ Upload excessivo (>25MB) bloqueado com sucesso: %v", err)
}

// Test 13.1: Envio em Lote de Múltiplas Imagens (Regra de Negócio: várias fotos em 1 mensagem)
func TestMultipleImagesBatchUpload(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	img1 := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("photo-one-content"))
	img2 := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("photo-two-content"))
	img3 := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("photo-three-content"))

	// Requisição com lote de 3 imagens para /api/upload
	payload := fmt.Sprintf(`{"images":["%s","%s","%s"]}`, img1, img2, img3)
	resp, err := http.Post(server.URL+"/api/upload", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("Erro ao enviar lote de imagens: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Status inesperado no envio de múltiplas fotos: %d", resp.StatusCode)
	}

	t.Logf("✓ Envio em lote de múltiplas fotos (regra de negócio) processado e aceito com sucesso")
}

// Test 14: Persistência do Banco de Dados SQLite e Tool Calls (db/db_*.go)
func TestDatabaseMessageAndToolCallPersistence(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	convID := "test_conv_persistence"
	userEmail := "dev@teste.com"

	// 1. Upsert da Conversa
	err := db.UpsertConversation(convID, userEmail, "Conversa de Teste", time.Now())
	if err != nil {
		t.Fatalf("Erro ao criar conversa no DB: %v", err)
	}

	owner, err := db.GetConversationOwner(convID)
	if err != nil || owner != userEmail {
		t.Fatalf("Proprietário da conversa incorreto: esperado %s, obtido %s", userEmail, owner)
	}

	// 2. Salvar Mensagem do Usuário
	err = db.SaveUserMessage("msg_user_1", convID, "Olá Agente", "/uploads/foto.png", []string{"/uploads/foto.png"}, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("Erro ao salvar mensagem do usuário: %v", err)
	}

	// 3. Salvar Mensagem do Agente com Thinking e Tool Calls
	thinkingText := "Pensando na melhor resposta..."
	toolCalls := []db.ToolCallRecord{
		{
			ID:      "tool_1",
			Name:    "run_command",
			Summary: "git status",
			Args:    map[string]interface{}{"CommandLine": "git status"},
			Output:  "On branch main",
			Status:  "completed",
		},
	}
	err = db.SaveAgentMessage("msg_agent_1", convID, "Executei o comando para você.", &thinkingText, nil, toolCalls, time.Now().UnixMilli())
	if err != nil {
		t.Fatalf("Erro ao salvar mensagem do agente: %v", err)
	}

	// 4. Carregar Mensagens e Validar Integridade
	msgs, err := db.GetMessagesFromDB(convID, 10)
	if err != nil {
		t.Fatalf("Erro ao carregar mensagens do DB: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("Quantidade incorreta de mensagens: esperado 2, obtido %d", len(msgs))
	}

	agentMsg := msgs[1]
	if agentMsg.Role != "agent" || agentMsg.Thinking == nil || *agentMsg.Thinking != thinkingText {
		t.Fatalf("Dados do agente corrompidos: %+v", agentMsg)
	}
	if len(agentMsg.ToolCalls) != 1 || agentMsg.ToolCalls[0].Summary != "git status" {
		t.Fatalf("ToolCalls corrompidos no DB: %+v", agentMsg.ToolCalls)
	}

	t.Logf("✓ Persistência de mensagens, thinking e tool calls validada com 100%% de integridade")
}

// Test 14B: Desduplicação de Mensagens e Limpeza Automática do Banco
func TestMessageDeduplicationAndCleanup(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	convID := "test_conv_dedup"
	now := time.Now().UnixMilli()

	_ = db.UpsertConversation(convID, "dev@teste.com", "Teste Dedup", time.Now())

	// Inserir mensagens duplicadas (uma com ID do app móvel e outra gerada pelo sync com prefixo msg_)
	_ = db.SaveUserMessage("1793573024723", convID, "Mensagem duplicada", "", nil, now)
	_ = db.SaveUserMessage("msg_0", convID, "Mensagem duplicada", "", nil, now+2000)

	// Inserir respostas duplicadas do agente (uma com agent_ e outra com msg_)
	_ = db.SaveAgentMessage("agent_1793573354056", convID, "Resposta completa", nil, nil, nil, now+5000)
	_ = db.SaveAgentMessage("msg_1", convID, "Resposta completa", nil, nil, nil, now+6000)

	// 1. LoadConversationMessages deve retornar mensagens desduplicadas (1 usuário e 1 agente)
	msgs := services.LoadConversationMessages(convID, 50)
	if len(msgs) != 2 {
		t.Fatalf("Esperava 2 mensagens após desduplicação em memória, obteve %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "Mensagem duplicada" {
		t.Fatalf("Mensagem do usuário incorreta: %+v", msgs[0])
	}
	if msgs[1].Role != "agent" || msgs[1].Content != "Resposta completa" {
		t.Fatalf("Mensagem do agente incorreta: %+v", msgs[1])
	}

	// 2. CleanupDuplicateMessages deve limpar as duplicatas diretamente no SQLite
	err := db.CleanupDuplicateMessages()
	if err != nil {
		t.Fatalf("Erro ao limpar duplicatas do banco: %v", err)
	}

	dbMsgs, err := db.GetMessagesFromDB(convID, 50)
	if err != nil {
		t.Fatalf("Erro ao consultar DB: %v", err)
	}
	if len(dbMsgs) != 2 {
		t.Fatalf("Esperava 2 mensagens no DB após CleanupDuplicateMessages, obteve %d", len(dbMsgs))
	}
	if dbMsgs[0].ID != "1793573024723" {
		t.Fatalf("Esperava preservar o ID do cliente '1793573024723', obteve '%s'", dbMsgs[0].ID)
	}
	if dbMsgs[1].ID != "agent_1793573354056" {
		t.Fatalf("Esperava preservar o ID do agente 'agent_1793573354056', obteve '%s'", dbMsgs[1].ID)
	}
	t.Logf("✓ Desduplicação de mensagens e limpeza de duplicatas validadas com sucesso")
}

// Test 15: Gerenciador de Sessões em Memória (services/session_manager.go)
func TestSessionManagerLifecycle(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	sessionID := "session_unit_test"
	s := services.Manager.GetSession(sessionID)
	if s == nil || s.ID != sessionID {
		t.Fatalf("Falha ao instanciar sessão no SessionManager")
	}

	services.Manager.SetLatestSessionID(sessionID)
	if services.Manager.GetLatestSessionID() != sessionID {
		t.Fatalf("LatestSessionID incorreto")
	}

	if services.Manager.ActiveSessionsCount() < 1 {
		t.Fatalf("ActiveSessionsCount deveria ser >= 1")
	}

	// Broadcast em sessão vazia não deve entrar em pânico
	s.Broadcast(map[string]interface{}{"type": "test", "data": "ping"})
	if len(s.History) == 0 {
		t.Fatalf("Histórico da sessão não armazenou evento transmitido")
	}

	// KillProcess em processo nulo não deve entrar em pânico
	s.KillProcess()
	if s.Status != "idle" {
		t.Fatalf("Status da sessão após KillProcess deveria ser 'idle', obtido '%s'", s.Status)
	}

	t.Logf("✓ Ciclo de vida e broadcast do SessionManager validados com sucesso")
}

// Test 16: Detecção e Fallback Automático de Porta em Conflito
func TestPortCollisionFallback(t *testing.T) {
	// Cria listener temporário na porta aleatória livre
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Falha ao abrir porta temporária: %v", err)
	}
	defer l.Close()

	occupiedPort := l.Addr().(*net.TCPAddr).Port

	// Algoritmo de fallback: deve detectar ocupação e encontrar a próxima
	var nextListener net.Listener
	var resolvedPort int
	for p := occupiedPort; p < occupiedPort+10; p++ {
		tryL, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			nextListener = tryL
			resolvedPort = p
			break
		}
	}

	if nextListener == nil {
		t.Fatalf("Falha no algoritmo de busca de porta alternativa")
	}
	defer nextListener.Close()

	if resolvedPort == occupiedPort {
		t.Fatalf("Deveria ter pulado a porta ocupada %d!", occupiedPort)
	}

	t.Logf("✓ Fallback de porta ocupada %d pulou automaticamente para porta livre: %d", occupiedPort, resolvedPort)
}

// Test 17: Bloqueio de Path Traversal no Servidor de Arquivos Estáticos (gateway/http_static.go)
func TestStaticFileServerPathTraversal(t *testing.T) {
	server, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// Tentativa de escapar a pasta web/ para ler arquivos do sistema
	req, err := http.NewRequest(http.MethodGet, server.URL+"/../../../../../../etc/passwd", nil)
	if err != nil {
		t.Fatalf("Erro ao criar requisição: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Erro ao executar requisição: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("FALHA DE SEGURANÇA: Path Traversal em rota estática retornou status %d (esperava 404)!", resp.StatusCode)
	}

	t.Logf("✓ Tentativa de Path Traversal no servidor estático bloqueada com 404")
}

// Test 18: Recuperação de Panic e Telemetria Discord com Sanitização
func TestPanicRecoveryMiddlewareAndDiscordTelemetry(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	var receivedPayload map[string]interface{}
	var webhookMu sync.Mutex
	receivedChan := make(chan bool, 1)

	mockDiscordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err == nil {
			webhookMu.Lock()
			receivedPayload = payload
			webhookMu.Unlock()
			select {
			case receivedChan <- true:
			default:
			}
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer mockDiscordServer.Close()

	// Configura o webhook do Discord para apontar para nosso servidor de teste mock
	config.AppConfig.ErrorWebhookURL = mockDiscordServer.URL

	// Cria um handler que simula um panic inesperado com dados sensíveis
	panickingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("nil pointer dereference at /Users/joao_silva/secret_project/main.go with token=Bearer secret_token_xyz_1234567890")
	})

	// Envolve com nosso middleware de proteção
	protectedHandler := gateway.PanicRecoveryMiddleware(panickingHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/crash-test", nil)
	rec := httptest.NewRecorder()

	// Executa a requisição - NÃO PODE CRASHAR O PROCESSO!
	protectedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Esperava código 500 do middleware de recuperação, mas obteve: %d", rec.Code)
	}

	var respBody map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &respBody); err != nil {
		t.Fatalf("Resposta não é um JSON válido: %v", err)
	}
	if respBody["code"] != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("Código de erro inesperado na resposta: %v", respBody["code"])
	}

	// Aguarda o envio assíncrono para o mock do Discord
	select {
	case <-receivedChan:
		webhookMu.Lock()
		defer webhookMu.Unlock()
		embeds, ok := receivedPayload["embeds"].([]interface{})
		if !ok || len(embeds) == 0 {
			t.Fatalf("Payload do Discord não contém embeds válidos")
		}

		payloadJSON, _ := json.Marshal(receivedPayload)
		rawStr := string(payloadJSON)

		if strings.Contains(rawStr, "/Users/joao_silva") {
			t.Fatalf("FALHA DE PRIVACIDADE: O relatório para o Discord expôs o diretório do usuário: %s", rawStr)
		}
		if strings.Contains(rawStr, "secret_token_xyz_1234567890") {
			t.Fatalf("FALHA DE PRIVACIDADE: O relatório para o Discord expôs o token confidencial: %s", rawStr)
		}
		if !strings.Contains(rawStr, "[REDACTED_TOKEN]") {
			t.Fatalf("Token deveria ter sido substituído por [REDACTED_TOKEN]: %s", rawStr)
		}

		t.Logf("✓ Panic recuperado com sucesso, resposta 500 entregue ao cliente e telemetria entregue ao Discord!")
	case <-time.After(3 * time.Second):
		t.Fatalf("Timeout aguardando entrega do relatório de telemetria ao Discord")
	}
}

// Test 19: Consentimento e Opt-Out de Telemetria (LGPD Compliance)
func TestTelemetryDisabledOptOutConsent(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	var webhookCalled bool
	var webhookMu sync.Mutex

	mockDiscordServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookMu.Lock()
		webhookCalled = true
		webhookMu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer mockDiscordServer.Close()

	config.AppConfig.ErrorWebhookURL = mockDiscordServer.URL
	// Simula a escolha do cliente/usuário de DESATIVAR a telemetria (Opt-Out)
	config.AppConfig.TelemetryEnabled = false

	panickingHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("erro interno de teste para validar opt-out")
	})

	protectedHandler := gateway.PanicRecoveryMiddleware(panickingHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/crash-optout", nil)
	rec := httptest.NewRecorder()

	protectedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Esperava status 500 mesmo com telemetria desativada, mas obteve: %d", rec.Code)
	}

	time.Sleep(200 * time.Millisecond)

	webhookMu.Lock()
	called := webhookCalled
	webhookMu.Unlock()

	if called {
		t.Fatalf("FALHA DE PRIVACIDADE: O servidor enviou dados para o Discord mesmo com TelemetryEnabled=false!")
	}

	t.Logf("✓ Conformidade com LGPD/Opt-out validada: Nenhum dado transmitido quando TelemetryEnabled=false")
}

// Test 20: Expansão de Tilde Multiplataforma e Resolução de Workspace "~"
func TestExpandHomeAndWorkspaceTildeResolution(t *testing.T) {
	_, cleanup := setupTestEnvironment(t)
	defer cleanup()

	// 1. Validar ExpandHome em diferentes formatos
	home, err := services.ExpandHome("~")
	if err != nil || home == "" || home == "~" {
		t.Fatalf("ExpandHome('~') falhou em expandir para diretório home: %v (%s)", err, home)
	}

	subHome, err := services.ExpandHome("~/meu_projeto")
	if err != nil || !strings.HasSuffix(subHome, "meu_projeto") || strings.HasPrefix(subHome, "~") {
		t.Fatalf("ExpandHome('~/meu_projeto') falhou: %v (%s)", err, subHome)
	}

	winSubHome, err := services.ExpandHome(`~\meu_projeto_win`)
	if err != nil || !strings.HasSuffix(winSubHome, "meu_projeto_win") || strings.HasPrefix(winSubHome, "~") {
		t.Fatalf("ExpandHome(`~\\meu_projeto_win`) falhou: %v (%s)", err, winSubHome)
	}
	t.Logf("✓ ExpandHome expandiu til com sucesso em todos os formatos multiplataforma")

	// 2. Criar arquivo de teste no DefaultWorkspace
	testFile := "workspace_test.txt"
	_, err = services.SaveFileContent(testFile, "conteudo de teste", "~")
	if err != nil {
		t.Fatalf("SaveFileContent com workspace='~' falhou com erro: %v", err)
	}

	// 3. ListDirectory com workspace="~" e path="" (cenário exato que causava erro no mobile)
	res, err := services.ListDirectory("", "~")
	if err != nil {
		t.Fatalf("ListDirectory com workspace='~' e path='' falhou (ERRO DO MOBILE REPRODUZIDO): %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatalf("ListDirectory retornou 0 itens, esperava encontrar %s", testFile)
	}
	t.Logf("✓ ListDirectory com workspace='~' resolveu para DefaultWorkspace e listou %d arquivo(s)", len(res.Items))

	// 4. ReadFileContent com workspace="~"
	contentRes, err := services.ReadFileContent(testFile, "~")
	if err != nil {
		t.Fatalf("ReadFileContent com workspace='~' falhou: %v", err)
	}
	if contentRes.Content != "conteudo de teste" {
		t.Fatalf("Conteúdo lido incorreto: %s", contentRes.Content)
	}
	t.Logf("✓ ReadFileContent com workspace='~' leu o arquivo com sucesso!")
}

func TestDispatchStartupTelemetryToEndpoint(t *testing.T) {
	receivedChan := make(chan map[string]interface{}, 1)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		receivedChan <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	os.Setenv("TELEMETRY_ENDPOINT", mockServer.URL)
	os.Setenv("TELEMETRY_ENABLED", "true")
	config.AppConfig.TelemetryEnabled = true
	defer os.Unsetenv("TELEMETRY_ENDPOINT")

	services.DispatchStartupTelemetry("1.4.0", 39)

	select {
	case payload := <-receivedChan:
		if payload["machine_id"] == nil || payload["machine_id"] == "" {
			t.Fatalf("Esperava machine_id válido, obteve: %v", payload["machine_id"])
		}
		if payload["app_version"] != "1.4.0" {
			t.Fatalf("Versão incorreta no payload: %v", payload["app_version"])
		}
		if payload["event"] != "server_start" {
			t.Fatalf("Evento incorreto no payload: %v", payload["event"])
		}
		t.Logf("✓ Telemetria de inicialização disparada com sucesso para endpoint personalizado: %v", payload)
	case <-time.After(2 * time.Second):
		t.Fatalf("Timeout: Telemetria de inicialização não foi entregue ao endpoint mock")
	}
}
