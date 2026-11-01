package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"antigravity-bridge/config"
	"antigravity-bridge/services"
)

func handleUpload(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20) // 100 MB para suporte a lote com múltiplas imagens

	var req struct {
		Image  string   `json:"image"`
		Images []string `json:"images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "JSON inválido ou limite de payload excedido"})
		return
	}

	var allImages []string
	if len(req.Images) > 0 {
		allImages = req.Images
	} else if req.Image != "" {
		allImages = append(allImages, req.Image)
	}

	if len(allImages) == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Nenhuma imagem fornecida"})
		return
	}

	var results []*services.UploadResult
	for _, imgStr := range allImages {
		res, err := services.SaveBase64Image(imgStr)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": fmt.Sprintf("Erro ao processar imagem: %v", err)})
			return
		}
		results = append(results, res)
	}

	if len(results) == 1 && len(req.Images) == 0 {
		respondJSON(w, http.StatusOK, results[0])
		return
	}

	var urls []string
	for _, r := range results {
		urls = append(urls, r.URL)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"uploads": results,
		"urls":    urls,
	})
}

func handleUploads(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	filename := filepath.Base(r.URL.Path)
	fullPath, exists := services.GetUploadFilePath(filename)
	if !exists {
		http.Error(w, "Upload not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, fullPath)
}

func handleMedia(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isAllowedOrigin(r) {
		http.Error(w, "Origem não permitida", http.StatusForbidden)
		return
	}
	rawPath := r.URL.Query().Get("path")
	fullPath, exists := services.GetMediaFilePath(rawPath)
	if !exists {
		http.Error(w, "Media not found", http.StatusNotFound)
		return
	}

	// Security: If accessing conversation/brain artifacts remotely, require authentication
	brainAbs, _ := filepath.Abs(config.AppConfig.BrainDir)
	if brainAbs != "" && strings.HasPrefix(fullPath, brainAbs) {
		if _, err := authenticateRequest(r); err != nil {
			http.Error(w, "Autenticação obrigatória", http.StatusUnauthorized)
			return
		}
	}

	w.Header().Set("Cache-Control", "public, max-age=86400")
	http.ServeFile(w, r, fullPath)
}
