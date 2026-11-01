package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"antigravity-bridge/config"
)

var (
	macHomePattern   = regexp.MustCompile(`/Users/[a-zA-Z0-9._-]+`)
	linuxHomePattern = regexp.MustCompile(`/home/[a-zA-Z0-9._-]+`)
	winHomePattern   = regexp.MustCompile(`(?i)[a-zA-Z]:\\Users\\[a-zA-Z0-9._-]+`)
	tokenPattern     = regexp.MustCompile(`(Bearer\s+|token=)[a-zA-Z0-9_\-\.]{15,}`)
)

func sanitizeTrace(raw string) string {
	if raw == "" {
		return ""
	}
	clean := macHomePattern.ReplaceAllString(raw, "~")
	clean = linuxHomePattern.ReplaceAllString(clean, "~")
	clean = winHomePattern.ReplaceAllString(clean, "~")
	clean = tokenPattern.ReplaceAllString(clean, "${1}[REDACTED_TOKEN]")
	return clean
}

type DiscordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type DiscordEmbedFooter struct {
	Text string `json:"text"`
}

type DiscordEmbed struct {
	Title       string              `json:"title"`
	Color       int                 `json:"color"`
	Description string              `json:"description,omitempty"`
	Fields      []DiscordEmbedField `json:"fields"`
	Footer      DiscordEmbedFooter  `json:"footer"`
	Timestamp   string              `json:"timestamp"`
}

type DiscordWebhookPayload struct {
	Username  string         `json:"username"`
	AvatarURL string         `json:"avatar_url,omitempty"`
	Embeds    []DiscordEmbed `json:"embeds"`
}

func isTelemetryEnabled() bool {
	envVal := strings.ToLower(strings.TrimSpace(os.Getenv("TELEMETRY_ENABLED")))
	if envVal == "false" || envVal == "0" || envVal == "no" {
		return false
	}
	return config.AppConfig.TelemetryEnabled
}

func getWebhookURL() string {
	if config.AppConfig.ErrorWebhookURL != "" {
		return config.AppConfig.ErrorWebhookURL
	}
	return os.Getenv("ERROR_WEBHOOK_URL")
}

func dispatchDiscordWebhook(embed DiscordEmbed) {
	if !isTelemetryEnabled() {
		return
	}

	webhookURL := getWebhookURL()
	if webhookURL == "" {
		return
	}

	payload := DiscordWebhookPayload{
		Username: "Antigravity Server Crash Monitor",
		Embeds:   []DiscordEmbed{embed},
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("[Telemetry] ⚠️ Falha ao entregar relatório ao Discord: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Println("[Telemetry] 📢 Relatório de erro transmitido com sucesso ao canal Discord!")
	} else {
		fmt.Printf("[Telemetry] ⚠️ Discord webhook retornou status %d\n", resp.StatusCode)
	}
}

// ReportCrash sends a high-priority crash / panic report to Discord asynchronously.
func ReportCrash(category, errStr, stackTrace string, contextData map[string]interface{}) {
	go func() {
		defer func() { _ = recover() }()

		cleanErr := sanitizeTrace(errStr)
		cleanStack := sanitizeTrace(stackTrace)
		if len(cleanStack) > 900 {
			cleanStack = cleanStack[:900] + "\n... [truncado]"
		}

		vData := LoadVersionData()
		envInfo := fmt.Sprintf("%s / %s", runtime.GOOS, runtime.GOARCH)

		var contextStr string
		if len(contextData) > 0 {
			b, _ := json.Marshal(contextData)
			contextStr = sanitizeTrace(string(b))
		}

		fields := []DiscordEmbedField{
			{Name: "🖥️ Ambiente (SO/Arch)", Value: envInfo, Inline: true},
			{Name: "📦 Versão", Value: fmt.Sprintf("v%s (#%d)", vData.Version, vData.Build), Inline: true},
			{Name: "🏷️ Categoria", Value: category, Inline: true},
			{Name: "❌ Erro Interceptado", Value: fmt.Sprintf("`%s`", cleanErr), Inline: false},
		}

		if contextStr != "" {
			fields = append(fields, DiscordEmbedField{
				Name:   "🔍 Contexto",
				Value:  fmt.Sprintf("```json\n%s\n```", contextStr),
				Inline: false,
			})
		}

		if cleanStack != "" {
			fields = append(fields, DiscordEmbedField{
				Name:   "📜 Stack Trace",
				Value:  fmt.Sprintf("```go\n%s\n```", cleanStack),
				Inline: false,
			})
		}

		embed := DiscordEmbed{
			Title:       "🚨 [CRASH / ERRO CRÍTICO] - Antigravity Server",
			Color:       0xEF4444, // Vermelho
			Description: "Um erro inesperado ocorreu no servidor do usuário e foi recuperado com segurança.",
			Fields:      fields,
			Footer:      DiscordEmbedFooter{Text: "Antigravity Telemetry System"},
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}

		dispatchDiscordWebhook(embed)
	}()
}

// ReportError sends a warning / operational error report to Discord.
func ReportError(category string, err error, contextData map[string]interface{}) {
	if err == nil {
		return
	}
	go func() {
		defer func() { _ = recover() }()

		cleanErr := sanitizeTrace(err.Error())
		vData := LoadVersionData()
		envInfo := fmt.Sprintf("%s / %s", runtime.GOOS, runtime.GOARCH)

		var contextStr string
		if len(contextData) > 0 {
			b, _ := json.Marshal(contextData)
			contextStr = sanitizeTrace(string(b))
		}

		fields := []DiscordEmbedField{
			{Name: "🖥️ Ambiente", Value: envInfo, Inline: true},
			{Name: "📦 Versão", Value: fmt.Sprintf("v%s (#%d)", vData.Version, vData.Build), Inline: true},
			{Name: "🏷️ Categoria", Value: category, Inline: true},
			{Name: "⚠️ Mensagem", Value: fmt.Sprintf("`%s`", cleanErr), Inline: false},
		}

		if contextStr != "" {
			fields = append(fields, DiscordEmbedField{
				Name:   "🔍 Detalhes",
				Value:  fmt.Sprintf("```json\n%s\n```", contextStr),
				Inline: false,
			})
		}

		embed := DiscordEmbed{
			Title:       "⚠️ [ALERTA OPERACIONAL] - Antigravity Server",
			Color:       0xF59E0B, // Amarelo/Laranja
			Description: "Ocorreu uma falha operacional no servidor que requer atenção.",
			Fields:      fields,
			Footer:      DiscordEmbedFooter{Text: "Antigravity Telemetry System"},
			Timestamp:   time.Now().UTC().Format(time.RFC3339),
		}

		dispatchDiscordWebhook(embed)
	}()
}
