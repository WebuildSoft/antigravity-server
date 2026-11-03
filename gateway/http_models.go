package gateway

import (
	"encoding/json"
	"net/http"

	"antigravity-bridge/services"
)

func handleModels(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	forceRefresh := r.URL.Query().Get("refresh") == "true"
	resp, err := services.FetchAvailableModels(forceRefresh)
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func handleActiveModel(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == http.MethodGet {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"activeModel": services.GetActiveModel(),
		})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Model == "" {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Modelo não informado"})
			return
		}

		if err := services.SetActiveModel(req.Model); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]interface{}{"error": err.Error()})
			return
		}

		respondJSON(w, http.StatusOK, map[string]interface{}{
			"success":     true,
			"activeModel": services.GetActiveModel(),
		})
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
