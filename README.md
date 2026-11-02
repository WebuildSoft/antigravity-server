<div align="center">

<img src="https://raw.githubusercontent.com/WebuildSoft/antigravity-mobile-site/main/icons/Icon-192.png" width="88" height="88" alt="Antigravity Mobile Logo" />

# Antigravity Mobile Server

Servidor companion de alta performance para conectar o aplicativo móvel **Antigravity Mobile** ao **Google Antigravity CLI (`agy`)**.

[![Go Version](https://img.shields.io/badge/go-1.24-00ADD8?style=flat-square&logo=go&logoColor=white)](go.mod)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-334155?style=flat-square)](#início-rápido)
[![Developer](https://img.shields.io/badge/developer-WebuildSoft-0284c7?style=flat-square)](https://github.com/WebuildSoft)

</div>

---

## Início Rápido

### Linux e macOS (Terminal)
Execute o script em linha única para instalação e inicialização:
```bash
curl -fsSL https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.sh | bash
```
*(Ou no repositório clonado: `./install.sh`)*

---

### Windows (PowerShell)
Execute no **PowerShell**:
```powershell
irm https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.ps1 | iex
```
*(Ou execute `.\start.ps1` no diretório extraído)*

---

## Funcionalidades do Instalador

* **Detecção Automática:** Identifica arquitetura e sistema operacional (Linux, macOS Universal ou Windows x64/ARM64).
* **Verificação do CLI Antigravity:** Valida se o binário `agy` está acessível no PATH do sistema.
* **Túnel Remoto Seguro:** Configura binário nativo do `cloudflared` para comunicação com terminação TLS sem dependência de portas expostas.
* **Pareamento Direto:** Gera código QR no próprio terminal para conexão instantânea com o aplicativo móvel.

---

## Diagnóstico e Solução de Problemas

### 1. Alerta do Windows Defender ou SmartScreen
* **Causa:** Binários de código aberto recém-compilados sem certificado comercial pago da Microsoft podem acionar alertas preventivos do sistema.
* **SmartScreen ("O Windows protegeu o seu computador"):**
  Selecione **"Mais informações"** e clique em **"Executar assim mesmo"**.
* **Arquivo bloqueado:**
  Acesse *Segurança do Windows > Proteção contra vírus e ameaças > Histórico de proteção* e clique em *Permitir no dispositivo*.
* **Recomendação técnica:**
  Execute diretamente o script PowerShell no terminal da pasta:
  ```powershell
  .\start.ps1
  ```

### 2. Autenticação com a Conta Google no Antigravity CLI (`agy`)
> [!WARNING]
> O comando `agy auth login` **não existe** no CLI e resultará em erro de sintaxe.

* Para autenticar sua conta, execute no terminal da sua máquina:
  ```bash
  agy
  ```
  O Antigravity abrirá uma sessão interativa e fornecerá o link de autorização do Google no navegador.
* Para validar a conectividade e os modelos de IA disponíveis:
  ```bash
  agy models
  ```

### 3. Conexão Local Bloqueada por Firewall (Porta 4040)
Caso o celular não localize o computador na rede Wi-Fi local, execute no **PowerShell como Administrador**:
```powershell
New-NetFirewallRule -DisplayName "Antigravity Server (Porta 4040)" -Direction Inbound -LocalPort 4040 -Protocol TCP -Action Allow
```
*(No Linux com UFW ativo: `sudo ufw allow 4040/tcp`)*.

### 4. Redes Públicas, Corporativas ou Conexão 4G/5G
Em redes com isolamento de cliente (AP Isolation) ou conexões fora da rede local, o servidor cria automaticamente um Quick Tunnel com terminação `.trycloudflare.com`. Aponte a câmera para o código QR do túnel para conectar de qualquer lugar.

---

## Configurações Opcionais (`.env`)

Copie o arquivo `.env.example` para `.env` para personalizar parâmetros de execução:
* `PORT`: Porta de escuta local (padrão: `4040`).
* `DEFAULT_WORKSPACE`: Diretório raiz padrão para navegação e comandos.
* `CLOUDFLARE_TUNNEL_TOKEN`: Token dedicado para túneis corporativos nomeados.
* `ALLOWED_EMAILS`: Lista restrita de e-mails autorizados para acesso via WebSocket.

---

## Estrutura do Projeto

```text
antigravity-server/
├── antigravity-server      # Binário Go compilado de alta performance
├── install.sh              # Script instalador para Linux e macOS
├── start.sh                # Inicializador de serviço com túnel e QR Code
├── install.ps1             # Script instalador nativo para Windows
├── start.ps1               # Inicializador de serviço para Windows
├── .env.example            # Template de variáveis de ambiente
├── web/                    # Interface web compilada (fallback)
├── bin/                    # Binários utilitários complementares
├── data/                   # Banco de dados SQLite local
└── uploads/                # Armazenamento de arquivos anexados
```

---

<div align="center">

Desenvolvido por WebuildSoft • Todos os direitos reservados © 2026

</div>
