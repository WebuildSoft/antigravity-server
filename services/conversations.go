package services

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
)

type ConversationItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updatedAt"`
	CreatedAt string `json:"createdAt"`
}

var (
	userReqRegex  = regexp.MustCompile(`(?s)<USER_REQUEST>(.*?)</USER_REQUEST>`)
	tagRegex      = regexp.MustCompile(`<[^>]+>`)
	photoTagRegex = regexp.MustCompile(`(?i)\[(?:Foto|Imagem) (?:enviada|anexada) pelo usu[áa]rio:\s*([^\]]+)\]`)
)

func ListConversationsForUser(userID string) []ConversationItem {
	if userID == "" {
		return ListConversations()
	}
	dbConvs, err := db.ListConversationsFromDB(userID)
	if err == nil {
		var list []ConversationItem
		for _, c := range dbConvs {
			list = append(list, ConversationItem{
				ID:        c.ID,
				Title:     c.Title,
				UpdatedAt: c.UpdatedAt,
				CreatedAt: c.CreatedAt,
			})
		}
		return list
	}
	return []ConversationItem{}
}

func ListConversations() []ConversationItem {
	// 1. Try fetching from SQLite first (ultra fast!)
	dbConvs, err := db.ListConversationsFromDB("")
	if err == nil && len(dbConvs) > 0 {
		var list []ConversationItem
		for _, c := range dbConvs {
			list = append(list, ConversationItem{
				ID:        c.ID,
				Title:     c.Title,
				UpdatedAt: c.UpdatedAt,
				CreatedAt: c.CreatedAt,
			})
		}
		return list
	}

	// 2. Fallback to scanning brain directory and trigger background SQLite sync
	brainDir := config.AppConfig.BrainDir
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return []ConversationItem{}
	}

	var conversations []ConversationItem

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		convID := entry.Name()
		if strings.HasPrefix(convID, ".") {
			continue
		}

		transcriptPath := filepath.Join(brainDir, convID, ".system_generated/logs/transcript.jsonl")
		fi, err := os.Stat(transcriptPath)
		if err != nil {
			continue
		}

		title := "Conversa " + convID
		if len(convID) >= 8 {
			title = "Conversa " + convID[:8]
		}
		createdAt := fi.ModTime()

		f, err := os.Open(transcriptPath)
		if err == nil {
			scanner := bufio.NewScanner(f)
			if scanner.Scan() {
				var raw map[string]interface{}
				if json.Unmarshal(scanner.Bytes(), &raw) == nil {
					if caStr, ok := raw["created_at"].(string); ok {
						if t, err := time.Parse(time.RFC3339, caStr); err == nil {
							createdAt = t
						}
					}
					if content, ok := raw["content"].(string); ok && content != "" {
						match := userReqRegex.FindStringSubmatch(content)
						if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
							title = strings.TrimSpace(match[1])
						} else {
							cleaned := strings.TrimSpace(tagRegex.ReplaceAllString(content, ""))
							if cleaned != "" {
								lines := strings.Split(cleaned, "\n")
								title = strings.TrimSpace(lines[0])
							}
						}
					}
				}
			}
			f.Close()
		}

		if len(title) > 70 {
			title = title[:67] + "..."
		}

		conversations = append(conversations, ConversationItem{
			ID:        convID,
			Title:     title,
			UpdatedAt: fi.ModTime().UTC().Format(time.RFC3339),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(conversations, func(i, j int) bool {
		return conversations[i].UpdatedAt > conversations[j].UpdatedAt
	})

	if len(conversations) > 40 {
		return conversations[:40]
	}
	return conversations
}
