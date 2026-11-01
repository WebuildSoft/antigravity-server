# 📋 Changelog - Antigravity Mobile & Server

Todas as alterações notáveis deste projeto serão documentadas neste arquivo.

O formato segue as diretrizes do [Keep a Changelog](https://keepachangelog.com/pt-BR/1.0.0/) e este projeto adere ao [Versionamento Semântico](https://semver.org/lang/pt-BR/).

---

## [1.4.0] - 2026-10-31 (Build 20)

### 📱 Aplicativo Mobile & Interface Web
- **Tratamento Elegante de Autenticação (`auth_error`):**
  - Implementado tratamento para o evento `auth_error` no WebSocket (`AntigravityBridge`), exibindo mensagens explicativas amigáveis no chat em vez de loops infinitos de reconexão.
- **Badge de Status de Diagnóstico e Privacidade (LGPD):**
  - Adicionado indicador visual de telemetria no diálogo de Configurações, informando ao usuário se o monitoramento de diagnósticos Discord está ativo e anonimizado.
- **Sincronização de Build Release:**
  - Compilação e atualização do pacote web e do binário Android `AntigravityMobile-v1.4.0.apk` (Build #20).

---

## [1.4.0] - 2026-10-31 (Build 19)

### 🛡️ Segurança (Backend Go - Auditoria Completa)
- **Correção de Leitura Arbitrária de Arquivos (Path Traversal / LFI):**
  - O endpoint `/api/media` e a função `GetMediaFilePath` agora restringem o acesso estritamente aos diretórios `uploads/` e `brain/`. Qualquer tentativa de ler arquivos fora dessas pastas (ex: `/etc`, `.ssh`, `.zsh_history`) é bloqueada com erro 404/403.
  - Correção na entrega de arquivos estáticos (`handleRootAndStatic`) com `filepath.Clean` e validação estrita de prefixo do diretório `web/`.
- **Proteção contra Cross-Site WebSocket Hijacking (CSWSH):**
  - Implementada validação do cabeçalho `Origin` no handshake do WebSocket (`isAllowedOrigin`), bloqueando requisições de sites externos maliciosos via navegador (HTTP 403) e aceitando apenas clientes autorizados (app nativo, localhost e domínios do túnel Cloudflare).
- **Correção de Bypass de Autenticação nas Conversas:**
  - Os endpoints `/api/conversations` e `/api/conversations/:id` agora exigem autenticação obrigatória (HTTP 401 para requisições remotas sem token) e validação de propriedade da conversa (HTTP 403 para usuários não autorizados).
- **Desacoplamento e Correção de Autorização (Zero-Config Mobile Login):**
  - Removido o bloqueio acidental onde o servidor inspecionava perfis locais do CLI (`~/.gemini/profiles`) e impedia logins legítimos feitos pelo app via Firebase.
  - O servidor agora aceita qualquer usuário autenticado que escanear o QR Code (`*`), mantendo suporte à restrição explícita no `.env` (`ALLOWED_EMAILS`) com alerta informativo no terminal caso um e-mail seja bloqueado.
- **Proteção de Endpoints Administrativos e Workspaces:**
  - Rotas de gerenciamento de contas (`/api/accounts`, `/api/accounts/switch`, `/api/accounts/import`, `/api/accounts/start-login`) e arquivos (`/api/workspace/tree`, `/api/workspace/file`, `/api/upload`) agora exigem autenticação ativa.
  - Validação de fronteiras de workspace em `resolveSafePath` e restrição de tipos de arquivos permitidos em `SaveFileContent`.
- **Auto-Configuração Inteligente de Portas (Port Fallback):**
  - Caso a porta padrão (`4040`) já esteja ocupada por outro processo no sistema, o servidor e o script `start.sh` detectam automaticamente o conflito, encontram a próxima porta livre (`4041`, `4042`, etc.), configuram o túnel Cloudflare e atualizam o QR Code instantaneamente sem travar.
- **Remoção de Credenciais Hardcoded do Código-Fonte:**
  - `GoogleOAuthClientID` e `GoogleOAuthClientSecret` foram completamente removidos do código Go e movidos para o arquivo de configuração seguro `.env`. O servidor agora faz leitura dinâmica via `config.AppConfig` e `loadEnvFile`.
- **Testes de Segurança e Arquitetura Automatizados:**
  - Expansão da suíte de testes automatizados (`tests/dev_test.go` e `tests/modules_test.go`) para **18 testes de ponta a ponta** divididos em 9 camadas:
    - Camada 1: Permissão para loopback Localhost.
    - Camada 2: Bloqueio estrito de requisições remotas sem token ou com token forjado.
    - Camada 3: Limite de upload de 25MB por foto e suporte a envio em lote de múltiplas imagens (até 100MB total).
    - Camada 4: Isolamento total de conversas entre diferentes usuários.
    - Camada 5: Hardening HTTP (405 Method Not Allowed, cabeçalhos nosniff/SAMEORIGIN, timeouts anti-Slowloris e limite de body via MaxBytesReader).
    - Camada 6: Proteção do workspace contra Path Traversal e leitura/gravação arbitrária de arquivos (LFI).
    - Camada 7: Persistência de conversas, mensagens, thinking e chamadas de ferramentas (*tool calls*) no SQLite WAL.
    - Camada 8: Resiliência, ciclo de vida do SessionManager e detecção/fallback automático de portas em conflito.
    - Camada 9: Telemetria de erros em tempo real via Discord Webhook com sanitização automática (redação de caminhos e tokens) e tolerância a falhas (Anti-Crash & Panic Recovery).
- **Telemetria de Erros & Recuperação de Pânico (Discord Webhooks):**
  - Implementado sistema de telemetria não-bloqueante (`services/telemetry.go`) integrado ao Discord via Webhooks.
  - Middleware global de captura de pânico (`gateway.PanicRecoveryMiddleware`) no HTTP e blocos `recover()` no WebSocket e no executor do agente CLI.
  - Stack traces e mensagens de erro são automaticamente higienizados por regex antes do envio (caminhos de diretórios pessoais e tokens de autenticação são substituídos por `~` e `[REDACTED_TOKEN]`), garantindo conformidade com a LGPD e privacidade total dos usuários.
  - **Consentimento & Opt-out (LGPD Compliance):** Suporte à flag `TELEMETRY_ENABLED=true/false` no `.env`, exposição do status de consentimento no handshake do WebSocket e no endpoint `/api/status`, com garantia comprovada por teste automatizado de que nenhum dado é transmitido quando desativado.
- **Auditoria e Hardening Exclusivo da Camada HTTP:**
  - **Mitigação contra Ataques Slowloris:** Configurados `ReadHeaderTimeout` (15s) e `IdleTimeout` (120s) na struct `http.Server` do Go nativo (`net/http`), prevenindo esgotamento de conexões abertas incompletas sem comprometer conexões WebSocket de longa duração.
  - **Proteção contra DoS de Memória (Payload Exhaustion):** Aplicado `http.MaxBytesReader` em todas as rotas de ingestão de dados (`/api/accounts/switch`: 64KB, `/api/accounts/exchange-code`: 64KB, `/api/accounts/import`: 2MB, `/api/workspace/file`: 10MB, `/api/upload`: 35MB), impedindo estouro de buffer por upload de arquivos ou JSON maliciosamente gigantescos.
  - **Validação Estrita de Métodos HTTP (405 Method Not Allowed):** Restrição precisa dos métodos HTTP em todas as 20 rotas públicas e privadas (bloqueando verbos não permitidos como POST/PUT em rotas exclusivamente GET/HEAD).
  - **Cabeçalhos de Segurança HTTP Universais:** Injeção padronizada de `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN` e `Referrer-Policy: strict-origin-when-cross-origin` em todas as respostas HTTP e pré-vôos CORS (`OPTIONS`).
  - **Proteção da Rota de Notificação de Versão:** Exigência de autenticação JWT com verificação de e-mail no endpoint `/api/version/notify`.
- **Modularização Arquitetural Completa (Clean Code, SOLID & DDD):**
  - Eliminação de todos os arquivos monolíticos do projeto em conformidade com o Princípio da Responsabilidade Única (SRP) e separação de Domínios (DDD):
    - **Camada Gateway:** `http.go` (179 linhas), `http_accounts.go`, `http_conversations.go`, `http_workspace.go`, `http_media.go`, `http_static.go`, `ws.go` (220 linhas) e `ws_handlers.go`.
    - **Camada Services:** `session_manager.go` (gerenciador de sessões e fila em memória), `agent_runner.go` (ciclo de vida de processos do `agy`), `agent_parser.go` (decodificação de streaming JSONL), `conversations.go` (listagem e metadados), `conversation_messages.go` (carregamento e paginação de mensagens), `oauth_client.go` (cliente HTTP Google OAuth2) e `accounts.go` (armazenamento e troca de credenciais locais).
    - **Camada Database:** `db.go` (conexão e schema WAL), `db_conversations.go` (queries de conversas e ownership) e `db_messages.go` (queries de mensagens e tool calls).
  - 100% de retrocompatibilidade mantida e 18/18 testes automatizados aprovados.

### ✨ Novas Funcionalidades (Mobile / UI)
- **Cópia no Chat:** Suporte a seleção de texto e botão para copiar mensagens e blocos de código diretamente no chat.
- **Login Google:** Botão rápido para copiar o link de autenticação nas configurações.
- **Rota Direta:** Criação do atalho `/login-google` para redirecionamento imediato para autorização no navegador.

---

## [1.3.9] - 2026-10-31 (Build 18)

### ✨ Adicionado
- **OAuth Remoto:** Autenticação Google OAuth executada diretamente do celular via navegador móvel.
- **Callback Automático:** Receptor local de callback para captura automática de credenciais e fallback manual para inserção de código.

---

## [1.3.8] - 2026-10-31 (Build 17)

### 🎨 Melhorias e Correções
- **Login Rápido no Mac:** Adicionar conta Google com abertura de terminal no macOS em 1 toque (`/api/accounts/start-login`).
- **Interface:** Melhor contraste visual nos botões da tela de gerenciamento de contas e mensagens informativas claras de validação.

---

## [1.3.7] - 2026-10-31 (Build 16)

### ✨ Adicionado
- **Multi-Contas Google:** Suporte a gerenciamento de múltiplos perfis de contas Google conectadas ao `agy`.
- **Troca Instantânea:** Alternância de perfil ativa em tempo real sem necessidade de reiniciar o servidor.
- **Monitoramento de Cotas:** Leitura e exibição em tempo real de modelos e limites de cota por conta.

---

## [1.3.6] - 2026-10-30 (Build 15)

### 🚀 Arquitetura e Backend
- **Migração para Go:** Substituição do backend legado por um servidor Go de alta performance.
- **Banco de Dados SQLite:** Persistência local ultra-rápida para conversas, histórico de mensagens e chamadas de ferramentas (*tool calls*).
- **Túnel Cloudflare Zero-Config:** Criação automática de túnel seguro temporário (`*.trycloudflare.com`) e exibição de QR Code no terminal via `start.sh`.
