package db

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type ToolCallRecord struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Summary string                 `json:"summary"`
	Args    map[string]interface{} `json:"args"`
	Output  string                 `json:"output,omitempty"`
	Status  string                 `json:"status"`
}

type MessageRecord struct {
	ID        string           `json:"id"`
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Thinking  *string          `json:"thinking"`
	Timestamp int64            `json:"timestamp"`
	ImageURL  *string          `json:"imageUrl"`
	ImageURLs []string         `json:"imageUrls"`
	ToolCalls []ToolCallRecord `json:"toolCalls"`
}

func FormatToolSummary(name string, args map[string]interface{}) string {
	if args != nil {
		if summary, ok := args["toolSummary"].(string); ok && strings.TrimSpace(summary) != "" {
			return strings.Trim(strings.TrimSpace(summary), "\"")
		}
	}
	cleanName := strings.ToLower(name)
	filePath := ""
	if args != nil {
		if p, ok := args["Path"].(string); ok {
			filePath = p
		} else if p, ok := args["path"].(string); ok {
			filePath = p
		}
	}

	baseName := filepath.Base(filePath)
	if strings.Contains(cleanName, "read_file") {
		if filePath != "" {
			return "Ler " + baseName
		}
		return "Leitura de arquivo"
	}
	if strings.Contains(cleanName, "edit_file") {
		if filePath != "" {
			return "Editar " + baseName
		}
		return "Edição de arquivo"
	}
	if strings.Contains(cleanName, "write_file") {
		if filePath != "" {
			return "Criar " + baseName
		}
		return "Criação de arquivo"
	}
	if strings.Contains(cleanName, "run_command") {
		if args != nil {
			if cmd, ok := args["CommandLine"].(string); ok && cmd != "" {
				return cmd
			}
		}
		return "Comando de terminal"
	}
	if strings.Contains(cleanName, "search_web") {
		if args != nil {
			if q, ok := args["query"].(string); ok && q != "" {
				return "Buscar: " + q
			}
		}
		return "Pesquisa web"
	}
	if strings.Contains(cleanName, "manage_task") {
		if args != nil {
			if act, ok := args["Action"].(string); ok && act != "" {
				return "Tarefa (" + act + ")"
			}
		}
		return "Gerenciar tarefas"
	}
	if strings.Contains(cleanName, "subagent") {
		if args != nil {
			if r, ok := args["Role"].(string); ok && r != "" {
				return "Agente: " + r
			}
			if t, ok := args["TypeName"].(string); ok && t != "" {
				return "Agente: " + t
			}
		}
		return "Subagente"
	}
	if filePath != "" {
		return baseName
	}
	if name != "" {
		return name
	}
	return "Ferramenta"
}

func SaveUserMessage(id, convID, content, imageURL string, imageUrls []string, timestamp int64) error {
	if id == "" {
		id = fmt.Sprintf("msg_%d", timestamp)
	}
	t := time.UnixMilli(timestamp).UTC()
	var imgsJSON string
	if len(imageUrls) > 0 {
		b, _ := json.Marshal(imageUrls)
		imgsJSON = string(b)
	}

	query := `
	INSERT OR REPLACE INTO messages (id, conversation_id, role, content, thinking, image_url, image_urls, created_at)
	VALUES (?, ?, 'user', ?, NULL, ?, ?, ?)
	`
	var imgPtr *string
	if imageURL != "" {
		imgPtr = &imageURL
	}
	var imgsPtr *string
	if imgsJSON != "" {
		imgsPtr = &imgsJSON
	}

	_, err := DB.Exec(query, id, convID, content, imgPtr, imgsPtr, t)
	return err
}

func SaveAgentMessage(id, convID, content string, thinking, imageURL *string, toolCalls []ToolCallRecord, timestamp int64) error {
	if id == "" {
		id = fmt.Sprintf("msg_%d", timestamp)
	}
	t := time.UnixMilli(timestamp).UTC()

	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	msgQuery := `
	INSERT OR REPLACE INTO messages (id, conversation_id, role, content, thinking, image_url, image_urls, created_at)
	VALUES (?, ?, 'agent', ?, ?, ?, NULL, ?)
	`
	_, err = tx.Exec(msgQuery, id, convID, content, thinking, imageURL, t)
	if err != nil {
		return err
	}

	for _, tc := range toolCalls {
		argsBytes, _ := json.Marshal(tc.Args)
		tcQuery := `
		INSERT OR REPLACE INTO tool_calls (id, message_id, name, summary, args, output, status)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		_, err = tx.Exec(tcQuery, tc.ID, id, tc.Name, tc.Summary, string(argsBytes), tc.Output, tc.Status)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func GetMessagesFromDB(convID string, limit int) ([]MessageRecord, error) {
	msgQuery := `
	SELECT id, role, content, thinking, image_url, image_urls, created_at
	FROM messages
	WHERE conversation_id = ?
	ORDER BY created_at ASC
	`
	rows, err := DB.Query(msgQuery, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []MessageRecord
	msgIndices := make(map[string]int)

	for rows.Next() {
		var m MessageRecord
		var ca time.Time
		var rawImgs *string

		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Thinking, &m.ImageURL, &rawImgs, &ca); err == nil {
			m.Timestamp = ca.UnixMilli()
			m.ToolCalls = make([]ToolCallRecord, 0)
			if rawImgs != nil && *rawImgs != "" {
				_ = json.Unmarshal([]byte(*rawImgs), &m.ImageURLs)
			}
			msgIndices[m.ID] = len(messages)
			messages = append(messages, m)
		}
	}

	if len(messages) == 0 {
		return messages, nil
	}

	// Fetch tool calls for these messages
	tcQuery := `
	SELECT tc.id, tc.message_id, tc.name, tc.summary, tc.args, tc.output, tc.status
	FROM tool_calls tc
	JOIN messages m ON tc.message_id = m.id
	WHERE m.conversation_id = ?
	ORDER BY tc.rowid ASC
	`
	tcRows, err := DB.Query(tcQuery, convID)
	if err == nil {
		defer tcRows.Close()
		for tcRows.Next() {
			var tc ToolCallRecord
			var msgID string
			var rawArgs *string
			if err := tcRows.Scan(&tc.ID, &msgID, &tc.Name, &tc.Summary, &rawArgs, &tc.Output, &tc.Status); err == nil {
				if rawArgs != nil && *rawArgs != "" {
					_ = json.Unmarshal([]byte(*rawArgs), &tc.Args)
				}
				if idx, ok := msgIndices[msgID]; ok {
					messages[idx].ToolCalls = append(messages[idx].ToolCalls, tc)
				}
			}
		}
	}

	if limit > 0 && len(messages) > limit {
		return messages[len(messages)-limit:], nil
	}
	return messages, nil
}

func GetLastMessageTime(convID string) time.Time {
	var ca time.Time
	_ = DB.QueryRow("SELECT created_at FROM messages WHERE conversation_id = ? ORDER BY created_at DESC LIMIT 1", convID).Scan(&ca)
	return ca
}
