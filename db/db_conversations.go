package db

import (
	"database/sql"
	"fmt"
	"time"
)

type ConversationRecord struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	CreatedAt string `json:"createdAt"`
}

func UpsertConversation(id, userID, title string, updatedAt time.Time) error {
	if title == "" {
		title = "Nova Conversa"
	}
	now := time.Now().UTC()
	if updatedAt.IsZero() {
		updatedAt = now
	}

	query := `
	INSERT INTO conversations (id, user_id, title, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = CASE WHEN excluded.title != 'Nova Conversa' THEN excluded.title ELSE conversations.title END,
		updated_at = excluded.updated_at
	`
	_, err := DB.Exec(query, id, userID, title, now, updatedAt)
	return err
}

func GetConversationOwner(id string) (string, error) {
	if DB == nil {
		return "", fmt.Errorf("database not initialized")
	}
	var owner string
	err := DB.QueryRow("SELECT user_id FROM conversations WHERE id = ?", id).Scan(&owner)
	return owner, err
}

func ListConversationsFromDB(userID string) ([]ConversationRecord, error) {
	var rows *sql.Rows
	var err error

	if userID != "" {
		query := `
		SELECT id, title, created_at, updated_at
		FROM conversations
		WHERE user_id = ? OR user_id = 'user@antigravity' OR user_id = ''
		ORDER BY updated_at DESC
		LIMIT 50
		`
		rows, err = DB.Query(query, userID)
		// Retroactively associate legacy or default sessions with this user
		go func(uid string) {
			if DB != nil {
				_, _ = DB.Exec("UPDATE conversations SET user_id = ? WHERE user_id = 'user@antigravity' OR user_id = ''", uid)
			}
		}(userID)
	} else {
		query := `
		SELECT id, title, created_at, updated_at
		FROM conversations
		ORDER BY updated_at DESC
		LIMIT 50
		`
		rows, err = DB.Query(query)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ConversationRecord
	for rows.Next() {
		var c ConversationRecord
		var ca, ua time.Time
		if err := rows.Scan(&c.ID, &c.Title, &ca, &ua); err == nil {
			c.CreatedAt = ca.UTC().Format(time.RFC3339)
			c.UpdatedAt = ua.UTC().Format(time.RFC3339)
			results = append(results, c)
		}
	}
	return results, nil
}
