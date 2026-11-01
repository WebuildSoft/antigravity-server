# 🚀 Antigravity Mobile Server

Servidor autônomo para conectar o aplicativo móvel **Antigravity Mobile** (Android / Google Play Store e Web) ao poder do **Google Antigravity CLI (`agy`)**.

---

## ⚡ Início Rápido (Quickstart)

### 🐧 Linux & 🍎 macOS (Terminal)
Execute em uma linha para instalar e iniciar automaticamente:
```bash
curl -fsSL https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.sh | bash
```
*(Ou se já clonou: `./install.sh`)*

---

### 🪟 Windows (PowerShell)
Abra o **PowerShell** e cole o comando de linha única:
```powershell
irm https://raw.githubusercontent.com/WebuildSoft/antigravity-server/main/install.ps1 | iex
```
*(Ou dê dois cliques em `start.bat` se baixou o `.zip` ou execute `.\start.ps1`)*

---

### O que o instalador faz automaticamente:
* ✅ Detecta seu sistema (Linux, macOS ou Windows, x64 ou ARM);
* ✅ Verifica se você tem o CLI `agy` (se não tiver, instala ou configura);
* ✅ Baixa o binário oficial seguro do `cloudflared` para criar túnel remoto com criptografia;
* ✅ Prepara o banco SQLite local e variáveis de ambiente `.env`;
* ✅ Inicia o servidor e exibe o **QR Code** no terminal para parear com o app Android!

---

## 🛠️ Resolução de Dúvidas e Problemas Comuns

### 🛡️ 1. O Windows Defender ou Antivírus bloqueou o `start.bat`?
* **Motivo:** O binário é open source recente sem certificado comercial pago da Microsoft e o `.bat` usa `-ExecutionPolicy Bypass`.
* **Se o SmartScreen aparecer:** Clique em **"Mais informações"** > **"Executar assim mesmo"**.
* **Se o antivírus bloquear:** Acesse *Segurança do Windows > Proteção contra vírus e ameaças > Histórico de proteção > Ações > Restaurar*.
* **Melhor alternativa:** Abra a pasta no PowerShell e rode o script nativo diretamente:
  ```powershell
  .\start.ps1
  ```

---

### 🔑 2. Como conectar a conta Google no Antigravity CLI (`agy`)?
> 🛑 **Atenção:** O comando `agy auth login` **não existe** e retornará erro.

* Para conectar ou autorizar sua conta Google, digite no terminal do seu computador:
  ```bash
  agy
  ```
  O Antigravity abrirá e fornecerá a janela/link oficial do Google para autorizar seu acesso no navegador.
* **Como testar se está tudo certo:**
  Digite no terminal:
  ```bash
  agy models
  ```
  Se a lista de modelos de IA for exibida, sua máquina está pronta para responder ao celular!

---

### 🧱 3. O celular não conecta no Wi-Fi? (Liberar o Firewall na Porta 4040)
Se o firewall do Windows bloqueou a porta local `4040`, abra o **PowerShell como Administrador** e execute:
```powershell
New-NetFirewallRule -DisplayName "Antigravity Server (Porta 4040)" -Direction Inbound -LocalPort 4040 -Protocol TCP -Action Allow
```
*(No Linux com UFW ativo: `sudo ufw allow 4040/tcp`)*.

---

### 🌐 4. Usando fora de casa ou em Wi-Fi de Faculdade/Empresa (Túnel Cloudflare)
Se você estiver no 4G/5G ou em redes com **Isolamento de AP** (onde o roteador impede dispositivos de se comunicarem diretamente):
- O servidor gera automaticamente uma URL de túnel seguro com final `.trycloudflare.com`.
- Basta escanear o QR Code de túnel no celular para se conectar de qualquer lugar do mundo.

---

## ⚙️ Configurações Opcionais (`.env`)

Copie o `.env.example` para `.env` se desejar personalizar:
* `PORT`: Porta local (padrão: `4040`).
* `DEFAULT_WORKSPACE`: Pasta padrão onde o assistente opera.
* `CLOUDFLARE_TUNNEL_TOKEN`: Caso você possua um domínio próprio e túnel configurado na Cloudflare.
* `ALLOWED_EMAILS`: Lista de e-mails autorizados (padrão: qualquer e-mail conectado ao `agy`).

---

## 📁 Estrutura de Arquivos

```text
antigravity-server/
├── antigravity-server      # Binário Go compilado de alta performance
├── install.sh              # Script instalador automático
├── start.sh                # Inicializador com túnel e QR Code
├── .env.example            # Exemplo de configurações opcionais
├── web/                    # Interface web compilada (fallback)
├── bin/                    # Binários utilitários (cloudflared)
├── data/                   # Banco de dados SQLite local
└── uploads/                # Armazenamento de arquivos enviados
```
