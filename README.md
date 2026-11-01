# 🚀 Antigravity Mobile Server

Servidor autônomo para conectar o aplicativo móvel **Antigravity Mobile** (Android / Google Play Store e Web) ao poder do **Google Antigravity CLI (`agy`)**.

---

## ⚡ Início Rápido (Quickstart)

### 1. Instalação e Checagem Automática
Abra o terminal na pasta do servidor e execute:
```bash
./install.sh
```
O instalador cuidará de tudo:
* ✅ Detecta seu sistema (Linux ou macOS, Intel/AMD ou ARM);
* ✅ Verifica se você tem o CLI `agy` (se não tiver, baixa e instala automaticamente);
* ✅ Verifica se você tem uma conta Google conectada (se não tiver, abre o login interativo);
* ✅ Baixa o `cloudflared` para criar conexões remotas seguras;
* ✅ Configura permissões e prepara o servidor.

---

### 2. Iniciar o Servidor
Execute:
```bash
./start.sh
```

Ao iniciar:
1. Um **Túnel Seguro Gratuito** (`*.trycloudflare.com`) é gerado instantaneamente (sem precisar de conta ou domínio).
2. Um **QR Code** é exibido diretamente no terminal.
3. No seu celular Android, abra o **Antigravity Mobile** e escaneie o QR Code para conectar!

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
