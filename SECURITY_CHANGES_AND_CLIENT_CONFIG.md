# 🛡️ Relatório de Segurança e Guia de Configuração do Cliente

> **Data da Auditoria e Correção:** 31 de Outubro de 2026  
> **Servidor:** Antigravity Mobile Server / Bridge (Go v2.0)  
> **Versão do App Mobile / APK:** `v1.4.0` (Build 19)  
> **Arquivo de Versão:** `version.json`  
> **APK:** `AntigravityMobile-v1.4.0.apk`  

---

## 📌 Sumário Executivo

Foram identificadas e corrigidas vulnerabilidades de segurança críticas na ponte de comunicação entre o aplicativo móvel (**Antigravity Mobile**) e o CLI local do Google Antigravity (**`agy`**), incluindo uma auditoria e blindagem dedicada na camada de servidor HTTP Go nativo (`net/http`).

Todas as vulnerabilidades foram sanadas no código Go, a suíte de testes automatizados foi expandida de 4 para **11 testes de segurança** (todos aprovados), e o binário executável `antigravity-server` foi recompilado com sucesso.

---

## 🔒 Detalhamento das Vulnerabilidades Corrigidas

### 1. Leitura Arbitrária de Arquivos (Path Traversal / LFI) em `/api/media` e `/`
* **Arquivos alterados:** [services/upload.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/services/upload.go) e [gateway/http.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/http.go)
* **O que ocorria:** 
  - O endpoint `/api/media?path=...` aceitava qualquer caminho absoluto do sistema operacional e usava `http.ServeFile` sem validar se o arquivo pertencia ao diretório da aplicação.
  - O manipulador de arquivos estáticos da interface web (`handleRootAndStatic`) permitia caminhos relativos maliciosos (`/../../`).
* **Como foi corrigido:**
  - `GetMediaFilePath` agora valida rigorosamente se o caminho absoluto do arquivo está contido **estritamente** dentro do diretório de uploads (`uploads/`) ou dentro do diretório do agente (`brain/`). Qualquer tentativa de acessar `/etc`, `.ssh`, `.zsh_history` ou arquivos fora desses diretórios é bloqueada com erro 404/403.
  - Arquivos do `brain` contendo mídia de conversas passam por validação de autenticação remota.
  - `handleRootAndStatic` agora valida a fronteira do diretório `web/` com `filepath.Clean` e prefixo seguro.

---

### 2. Cross-Site WebSocket Hijacking (CSWSH) e Proteção de Origem
* **Arquivos alterados:** [gateway/ws.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/ws.go) e [gateway/http.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/http.go)
* **O que ocorria:**
  - `websocket.Upgrader` usava `CheckOrigin: func(r *http.Request) bool { return true }`, permitindo que sites maliciosos acessados no navegador abrissem conexões WebSocket para `ws://localhost:4040/ws`.
  - Como conexões via localhost eram tratadas como locais sem token inicial, um script em segundo plano poderia injetar comandos no agente via `action: "send"`.
* **Como foi corrigido:**
  - Criada a função `isAllowedOrigin(r *http.Request)` que valida rigorosamente o cabeçalho `Origin`.
  - Permite apenas: clientes nativos móveis (sem header Origin), `localhost`, `127.0.0.1`, domínios autorizados do túnel Cloudflare (`*.trycloudflare.com`) e o host configurado em `PUBLIC_URL`. Origens web desconhecidas são rejeitadas imediatamente no handshake (HTTP 403).
  - Todas as ações sensíveis no WebSocket (`switch_account`, `import_account`, `remove_account`, `save_file`) agora exigem autenticação ativa.

---

### 3. Falha Lógica e Bypass de Autenticação nas Conversas
* **Arquivos alterados:** [gateway/http.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/http.go) e [services/conversations.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/services/conversations.go)
* **O que ocorria:**
  - Em `/api/conversations` e `/api/conversations/:id`, se a requisição não enviasse o token, a variável ficava vazia e o sistema retornava a lista completa de conversas ou as mensagens sem validar permissão.
* **Como foi corrigido:**
  - Implementada a função unificada `authenticateRequest(r *http.Request)`.
  - Requisições remotas sem token ou com token inválido agora recebem **HTTP 401 Unauthorized**.
  - Acesso a conversas de outro usuário recebe **HTTP 403 Forbidden**.

---

### 4. Endpoints de Workspace e Contas sem Autenticação
* **Arquivos alterados:** [gateway/http.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/http.go) e [services/file.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/services/file.go)
* **O que ocorria:**
  - `/api/workspace/tree`, `/api/workspace/file`, `/api/upload` e rotas `/api/accounts/*` podiam ser acessadas e manipuladas sem token.
* **Como foi corrigido:**
  - Todas essas rotas agora chamam `authenticateRequest(r)` obrigatoriamente.
  - Em `services/file.go`, `resolveSafePath` agora valida que o workspace solicitado deve estar estritamente contido no `DEFAULT_WORKSPACE` permitido, e `SaveFileContent` bloqueia extensões não autorizadas.

---

### 5. Auditoria e Hardening da Camada HTTP Nativa (net/http)
* **Arquivos alterados:** [main.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/main.go), [gateway/http.go](file:///Users/helenilsoncorreadeoliveira/Documents/antigravity-server/gateway/http.go)
* **O que ocorria:**
  - O servidor Go vanilla não configurava timeouts para cabeçalhos HTTP, ficando vulnerável a ataques Slowloris (esgotamento de descritores de arquivo e conexões abertas incompletas).
  - Rotas com parsing de JSON/Multipart no body não impunham limites máximos de tamanho (`r.Body` irrestrito), permitindo ataques de DoS por esgotamento de memória RAM.
  - Endpoints não validavam o método HTTP (`GET`, `POST`, `HEAD`), permitindo métodos arbitrários e comportamento inesperado.
  - O endpoint de notificação `/api/version/notify` não possuía validação de autenticação remota.
* **Como foi corrigido:**
  - `main.go`: Configurados `ReadHeaderTimeout: 15 * time.Second` e `IdleTimeout: 120 * time.Second` na struct `http.Server`.
  - `gateway/http.go`: Aplicado `http.MaxBytesReader` em todas as rotas receptoras de body (`handleAccountSwitch`, `handleAccountExchangeCode`, `handleAccountImport`, `handleWorkspaceFile`, `handleUpload`).
  - Validação estrita de verbos HTTP em todas as rotas (retornando `405 Method Not Allowed` para métodos não suportados).
  - Adicionados cabeçalhos universais de segurança (`X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`).
  - Protegido `/api/version/notify` com autenticação Firebase JWT.

---

## 📱 Guia de Configuração para os Clientes (APK Mobile e Web)

Para que o aplicativo móvel Flutter (**APK**) e o cliente **Web** se comuniquem perfeitamente com o servidor com as novas camadas de proteção ativas, siga as diretrizes abaixo:

### 1. Headers de Autenticação em Requisições HTTP REST
Em qualquer requisição HTTP feita pelo app Flutter (ex: em `project_file_service.dart` ou `antigravity_bridge.dart`), adicione o cabeçalho `Authorization`:

```dart
final token = await AuthService().getIdToken();

final headers = {
  'Content-Type': 'application/json',
  if (token != null && token.isNotEmpty) 'Authorization': 'Bearer $token',
};

final res = await http.get(url, headers: headers);
```

> **Alternativa (Query Parameter):**  
> Se preferir ou para download direto de arquivos, você também pode enviar o token como parâmetro de URL:  
> `GET /api/workspace/tree?path=...&token=<FIREBASE_ID_TOKEN>`

---

### 2. Conexão WebSocket
O aplicativo móvel já utiliza o parâmetro `?token=` no handshake do WebSocket:
```dart
final wsUrl = '$baseWsUrl/ws?token=$firebaseIdToken';
```
* **No Android / APK:** Como clientes nativos não enviam cabeçalho `Origin` do navegador, a conexão é aceita diretamente desde que o token Firebase seja válido.
* **No Cliente Web:** Quando rodar o Flutter Web no navegador, certifique-se de acessar o web client através da mesma URL do servidor ou túnel Cloudflare (ex: `https://xxxx.trycloudflare.com`), o que garante que o cabeçalho `Origin` seja aceito.

---

### 3. Sincronização de Versão do App
* **Versão ativa:** `1.4.0` (Build `19`)
* **Arquivo de metadados (`version.json`):**
```json
{
  "version": "1.4.0",
  "build": 19,
  "releasedAt": "2026-10-31T08:35:00Z",
  "changelog": "Função copiar mensagens e código no chat, botão copiar link de login e rota direta /login-google",
  "downloadUrl": "/baixar"
}
```
* **Download direto do APK:**
  - A rota `/app.apk`, `/baixar` e `/AntigravityMobile` entregam automaticamente o binário `AntigravityMobile-v1.4.0.apk`.

---

## 🧪 Validação dos Testes Automatizados

Para rodar a verificação de segurança a qualquer momento:
```bash
./test-dev.sh
```

**Resultado dos 18 testes executados (8 Camadas):**
1. ✅ `TestRemoteConnectionWithoutTokenRejected`: Bloqueio imediato de acesso remoto sem token (código 4001).
2. ✅ `TestInvalidTokenRejected`: Rejeição imediata de token inválido/forjado.
3. ✅ `TestLocalhostConnectionAccepted`: Permissão para loopback local do desenvolvedor.
4. ✅ `TestConversationIsolation`: Isolamento estrito de dados entre diferentes usuários.
5. ✅ `TestDisallowedOriginRejected`: Bloqueio de origens maliciosas externas (CSWSH) com HTTP 403.
6. ✅ `TestMediaArbitraryFileReadBlocked`: Bloqueio de tentativa de leitura de arquivos do sistema (`/etc/passwd`) com HTTP 404.
7. ✅ `TestRemoteConversationsWithoutTokenRejected`: Bloqueio remoto a `/api/conversations` sem token (HTTP 401).
8. ✅ `TestRemoteWorkspaceWithoutTokenRejected`: Bloqueio remoto ao explorador de arquivos sem token (HTTP 401).
9. ✅ `TestHTTPMethodsAndSecurityHeaders`: Validação de cabeçalhos de segurança (`nosniff`, `SAMEORIGIN`) e rejeição de métodos inválidos (HTTP 405).
10. ✅ `TestRequestBodySizeLimit`: Bloqueio de requisições com payload excessivo via `MaxBytesReader` (HTTP 400).
11. ✅ `TestRemoteVersionNotifyRequiresAuth`: Bloqueio de notificação remota de versão sem autenticação (HTTP 401).
12. ✅ `TestWorkspacePathTraversalBlocked`: Bloqueio de Path Traversal em leitura e gravação no workspace (`services/file.go`).
13. ✅ `TestUploadBase64ImageLimit`: Limite de upload de 25MB individual por imagem e proteção anti-DoS (`services/upload.go`).
14. ✅ `TestMultipleImagesBatchUpload`: Suporte a envio em lote de múltiplas fotos (regra de negócio da galeria mobile com limite global de 100MB).
15. ✅ `TestDatabaseMessageAndToolCallPersistence`: Persistência completa de conversas, thinking e tool calls no SQLite (`db/db_*.go`).
16. ✅ `TestSessionManagerLifecycle`: Ciclo de vida de sessões, broadcast e fila de execução (`services/session_manager.go`).
17. ✅ `TestPortCollisionFallback`: Algoritmo de busca e auto-fallback quando uma porta está ocupada.
18. ✅ `TestStaticFileServerPathTraversal`: Bloqueio de tentativa de fuga de diretório web para arquivos do sistema (`gateway/http_static.go`).
