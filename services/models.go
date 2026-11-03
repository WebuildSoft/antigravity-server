package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"antigravity-bridge/config"
)

type AiModelInfo struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Provider       string `json:"provider"` // "google", "anthropic", "openai"
	IsDefault      bool   `json:"isDefault"`
	Description    string `json:"description"`
	QuotaFamily    string `json:"quotaFamily"` // "Gemini Models" or "Claude and GPT models"
	QuotaRemaining int    `json:"quotaRemaining"`
}

type ModelsResponse struct {
	Models      []AiModelInfo `json:"models"`
	ActiveModel string        `json:"activeModel"`
	Timestamp   string        `json:"timestamp"`
	Cached      bool          `json:"cached"`
}

var (
	modelsCache        *ModelsResponse
	modelsCacheTime    time.Time
	modelsMutex        sync.RWMutex
	activeModelMutex   sync.RWMutex
	currentActiveModel = "gemini-3.8-flash-medium"
)

func getActiveModelFilePath() string {
	dataDir := filepath.Join(".", "data")
	_ = os.MkdirAll(dataDir, 0755)
	return filepath.Join(dataDir, "active_model.txt")
}

// GetActiveModel returns the currently chosen model ID.
func GetActiveModel() string {
	activeModelMutex.RLock()
	defer activeModelMutex.RUnlock()
	return currentActiveModel
}

// SetActiveModel updates the active AI model and notifies connected clients.
func SetActiveModel(modelID string) error {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return fmt.Errorf("modelo não especificado")
	}

	activeModelMutex.Lock()
	currentActiveModel = modelID
	activeModelMutex.Unlock()

	_ = os.WriteFile(getActiveModelFilePath(), []byte(modelID), 0644)

	modelsMutex.Lock()
	if modelsCache != nil {
		modelsCache.ActiveModel = modelID
	}
	modelsMutex.Unlock()

	fmt.Printf("[Models] 🤖 Modelo ativo atualizado para: %s\n", modelID)

	if Manager != nil {
		Manager.BroadcastAll(map[string]interface{}{
			"type":        "model_changed",
			"activeModel": modelID,
		})
	}

	return nil
}

// InitActiveModel loads the saved active model from disk if available.
func InitActiveModel() {
	fPath := getActiveModelFilePath()
	if b, err := os.ReadFile(fPath); err == nil {
		m := strings.TrimSpace(string(b))
		if m != "" {
			activeModelMutex.Lock()
			currentActiveModel = m
			activeModelMutex.Unlock()
			fmt.Printf("[Models] Modelo ativo carregado do disco: %s\n", m)
			return
		}
	}
	currentActiveModel = "gemini-3.8-flash-medium"
}

// FetchAvailableModels queries agy models with in-memory caching.
func FetchAvailableModels(forceRefresh bool) (*ModelsResponse, error) {
	if !forceRefresh {
		modelsMutex.RLock()
		if modelsCache != nil && time.Since(modelsCacheTime) < 10*time.Minute {
			resp := *modelsCache
			resp.Cached = true
			resp.ActiveModel = GetActiveModel()
			modelsMutex.RUnlock()
			return &resp, nil
		}
		modelsMutex.RUnlock()
	}

	defaultFallbackModels := []AiModelInfo{
		{
			ID:             "gemini-3.8-flash-medium",
			Name:           "Gemini 3.8 Flash (Medium)",
			Provider:       "google",
			IsDefault:      true,
			Description:    "Rápido e balanceado para código e raciocínio diário",
			QuotaFamily:    "Gemini Models",
			QuotaRemaining: 98,
		},
		{
			ID:             "gemini-3.8-flash-high",
			Name:           "Gemini 3.8 Flash (High)",
			Provider:       "google",
			IsDefault:      false,
			Description:    "Máxima profundidade de raciocínio Flash",
			QuotaFamily:    "Gemini Models",
			QuotaRemaining: 98,
		},
		{
			ID:             "gemini-3.8-flash-low",
			Name:           "Gemini 3.8 Flash (Low)",
			Provider:       "google",
			IsDefault:      false,
			Description:    "Ultrarrápido com baixo consumo de cota",
			QuotaFamily:    "Gemini Models",
			QuotaRemaining: 98,
		},
		{
			ID:             "claude-sonnet-4-6",
			Name:           "Claude Sonnet 4.6 (Thinking)",
			Provider:       "anthropic",
			IsDefault:      false,
			Description:    "Raciocínio avançado Anthropic com Thinking",
			QuotaFamily:    "Claude and GPT models",
			QuotaRemaining: 50,
		},
		{
			ID:             "claude-opus-4-6-thinking",
			Name:           "Claude Opus 4.6 (Thinking)",
			Provider:       "anthropic",
			IsDefault:      false,
			Description:    "Máxima capacidade analítica profunda",
			QuotaFamily:    "Claude and GPT models",
			QuotaRemaining: 50,
		},
		{
			ID:             "gpt-oss-120b-medium",
			Name:           "GPT-OSS 120B (Medium)",
			Provider:       "openai",
			IsDefault:      false,
			Description:    "Modelo aberto de alta performance",
			QuotaFamily:    "Claude and GPT models",
			QuotaRemaining: 50,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, config.AppConfig.AgyBin, "models")
	out, err := cmd.CombinedOutput()

	var models []AiModelInfo
	if err == nil && len(out) > 0 {
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "Fetching") || strings.HasPrefix(line, "Usage:") || strings.HasPrefix(line, "⠋") || strings.HasPrefix(line, "⠙") {
				continue
			}

			var id, name string
			if strings.Contains(line, "\t") {
				parts := strings.Split(line, "\t")
				if len(parts) >= 2 {
					id = strings.TrimSpace(parts[0])
					name = strings.TrimSpace(parts[1])
				}
			} else {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					id = parts[0]
					name = strings.Join(parts[1:], " ")
				}
			}

			if id != "" && name != "" {
				provider := "google"
				quotaFam := "Gemini Models"
				desc := "Modelo oficial Google Gemini"

				if strings.Contains(id, "claude") {
					provider = "anthropic"
					quotaFam = "Claude and GPT models"
					desc = "Raciocínio avançado Anthropic"
				} else if strings.Contains(id, "gpt") {
					provider = "openai"
					quotaFam = "Claude and GPT models"
					desc = "Modelo aberto de alta capacidade"
				}

				if strings.Contains(id, "high") {
					desc = "Máxima capacidade de raciocínio profundo"
				} else if strings.Contains(id, "low") {
					desc = "Ultrarrápido com baixo consumo de cota"
				} else if strings.Contains(id, "medium") && provider == "google" {
					desc = "Rápido e balanceado para código e tarefas gerais"
				}

				models = append(models, AiModelInfo{
					ID:          id,
					Name:        name,
					Provider:    provider,
					IsDefault:   id == "gemini-3.8-flash-medium",
					Description: desc,
					QuotaFamily: quotaFam,
				})
			}
		}
	}

	if len(models) == 0 {
		models = defaultFallbackModels
	}

	// Enrich with real quota data if available
	quotaData, _ := GetQuotaData(false)
	if quotaData != nil {
		for i := range models {
			for _, q := range quotaData.Items {
				isGeminiMatch := models[i].Provider == "google" && strings.Contains(strings.ToLower(q.Model), "gemini")
				isClaudeMatch := models[i].Provider != "google" && (strings.Contains(strings.ToLower(q.Model), "claude") || strings.Contains(strings.ToLower(q.Model), "gpt"))

				if isGeminiMatch || isClaudeMatch {
					if strings.Contains(strings.ToLower(q.LimitType), "five") || models[i].QuotaRemaining == 0 {
						models[i].QuotaRemaining = q.Remaining
					}
				}
			}
		}
	}

	resp := &ModelsResponse{
		Models:      models,
		ActiveModel: GetActiveModel(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Cached:      false,
	}

	modelsMutex.Lock()
	modelsCache = resp
	modelsCacheTime = time.Now()
	modelsMutex.Unlock()

	return resp, nil
}

// InvalidateModelsCache clears the cached models list.
func InvalidateModelsCache() {
	modelsMutex.Lock()
	modelsCache = nil
	modelsMutex.Unlock()
}
