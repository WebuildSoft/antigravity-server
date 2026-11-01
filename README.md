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
*(Ou dê dois cliques em `start.bat` se baixou o `.zip`)*

---

### O que o instalador faz automaticamente:
* ✅ Detecta seu sistema (Linux, macOS ou Windows, x64 ou ARM);
* ✅ Verifica se você tem o CLI `agy` (se não tiver, instala ou configura);
* ✅ Baixa o binário oficial seguro do `cloudflared` para criar túnel remoto com criptografia;
* ✅ Prepara o banco SQLite local e variáveis de ambiente `.env`;
* ✅ Inicia o servidor e exibe o **QR Code** no terminal para parear com o app Android!

---

### 3. Rodar Testes de Segurança em Modo Dev
Para validar se as camadas de proteção estão ativas e funcionando:
```bash
./test-dev.sh
```

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
