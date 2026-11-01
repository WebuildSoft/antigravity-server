package gateway

import (
	"net/http"
	"strings"

	"antigravity-bridge/db"
	"antigravity-bridge/services"
)

func handleConversations(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user, err := authenticateRequest(r)
	if err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}

	filterEmail := user.Email
	if user.UID == "local-dev" {
		filterEmail = ""
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"conversations": services.ListConversationsForUser(filterEmail),
	})
}

func handleSingleConversation(w http.ResponseWriter, r *http.Request) {
	enableCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	convID := strings.TrimPrefix(r.URL.Path, "/api/conversations/")
	convID = strings.TrimSpace(convID)

	user, err := authenticateRequest(r)
	if err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]interface{}{"error": "Autenticação obrigatória: " + err.Error()})
		return
	}

	if owner, err := db.GetConversationOwner(convID); err == nil && owner != "" {
		if user.UID != "local-dev" && owner != user.Email && owner != "user@antigravity" && owner != "*" {
			respondJSON(w, http.StatusForbidden, map[string]interface{}{"error": "Acesso não autorizado a esta conversa."})
			return
		}
	}

	messages := services.LoadConversationMessages(convID, 0)
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"conversationId": convID,
		"messages":       messages,
	})
}
