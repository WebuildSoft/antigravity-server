package gateway

import (
	"encoding/json"
	"net/http"

	"antigravity-bridge/config"
	"antigravity-bridge/services"
)

func handleWorkspaceTree(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false,
			"error":   "Autenticação obrigatória: " + err.Error(),
		})
		return
	}
	ws := r.URL.Query().Get("workspace")
	if ws == "" {
		ws = config.AppConfig.DefaultWorkspace
	}
	p := r.URL.Query().Get("path")

	result, err := services.ListDirectory(p, ws)
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":     true,
		"currentPath": result.CurrentPath,
		"workspace":   result.Workspace,
		"items":       result.Items,
	})
}

func handleWorkspaceFile(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if _, err := authenticateRequest(r); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{
			"success": false,
			"error":   "Autenticação obrigatória: " + err.Error(),
		})
		return
	}

	if r.Method == http.MethodGet {
		ws := r.URL.Query().Get("workspace")
		if ws == "" {
			ws = config.AppConfig.DefaultWorkspace
		}
		p := r.URL.Query().Get("path")
		if p == "" {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Path is required"})
			return
		}
		result, err := services.ReadFileContent(p, ws)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success":     true,
			"path":        result.Path,
			"content":     result.Content,
			"size":        result.Size,
			"modified":    result.Modified,
			"isEditable":  result.IsEditable,
			"currentPath": result.CurrentPath,
		})
		return
	}

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		var req struct {
			Path      string `json:"path"`
			Content   string `json:"content"`
			Workspace string `json:"workspace"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Invalid body"})
			return
		}
		ws := req.Workspace
		if ws == "" {
			ws = config.AppConfig.DefaultWorkspace
		}
		if req.Path == "" {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"success": false, "error": "Path is required"})
			return
		}
		result, err := services.SaveFileContent(req.Path, req.Content, ws)
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{
				"success": false,
				"error":   err.Error(),
			})
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success":     true,
			"path":        result.Path,
			"content":     result.Content,
			"size":        result.Size,
			"modified":    result.Modified,
			"isEditable":  result.IsEditable,
			"currentPath": result.CurrentPath,
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
