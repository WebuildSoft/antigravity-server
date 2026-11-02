package db

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"antigravity-bridge/config"
)

var (
	userReqRegex  = regexp.MustCompile(`(?s)<USER_REQUEST>(.*?)</USER_REQUEST>`)
	tagRegex      = regexp.MustCompile(`<[^>]+>`)
	photoTagRegex = regexp.MustCompile(`(?i)\[(?:Foto|Imagem) (?:enviada|anexada) pelo usu[áa]rio:\s*([^\]]+)\]`)
)

func defaultUser() string {
	if len(config.AppConfig.AllowedEmails) > 0 && config.AppConfig.AllowedEmails[0] != "*" {
		return config.AppConfig.AllowedEmails[0]
	}
	return "user@antigravity"
}

func SyncLegacyBrainTranscripts() {
	brainDir := config.AppConfig.BrainDir
	entries, err := os.ReadDir(brainDir)
	if err != nil {
		return
	}

	importedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		convID := entry.Name()
		transcriptPath := filepath.Join(brainDir, convID, ".system_generated/logs/transcript.jsonl")
		fi, err := os.Stat(transcriptPath)
		if err != nil {
			continue
		}

		title := "Conversa " + convID
		if len(convID) >= 8 {
			title = "Conversa " + convID[:8]
		}

		f, err := os.Open(transcriptPath)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		buf := make([]byte, 0, 512*1024)
		scanner.Buffer(buf, 10*1024*1024)

		var firstTitle string
		for scanner.Scan() {
			var raw map[string]interface{}
			if err := json.Unmarshal(scanner.Bytes(), &raw); err == nil {
				if content, ok := raw["content"].(string); ok && content != "" {
					match := userReqRegex.FindStringSubmatch(content)
					if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
						firstTitle = strings.TrimSpace(match[1])
						break
					} else {
						cleaned := strings.TrimSpace(tagRegex.ReplaceAllString(content, ""))
						if cleaned != "" {
							lines := strings.Split(cleaned, "\n")
							firstTitle = strings.TrimSpace(lines[0])
							break
						}
					}
				}
			}
		}
		f.Close()

		if firstTitle != "" {
			if len(firstTitle) > 70 {
				firstTitle = firstTitle[:67] + "..."
			}
			title = firstTitle
		}

		_ = UpsertConversation(convID, defaultUser(), title, fi.ModTime())
		importedCount++
	}

	if importedCount > 0 {
		fmt.Printf("[DB] Synced %d conversations from brain into SQLite\n", importedCount)
	}
}

func ImportSingleConversationTranscript(convID string) {
	transcriptPath := filepath.Join(config.AppConfig.BrainDir, convID, ".system_generated/logs/transcript.jsonl")
	file, err := os.Open(transcriptPath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 512*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var currentAgentMsgID string
	var currentAgentContent string
	var currentAgentThinking *string
	var currentAgentImg *string
	var currentAgentTCs []ToolCallRecord
	var currentAgentTime int64

	flushAgent := func() {
		if currentAgentMsgID != "" {
			// Check if this agent message already exists in DB (e.g. saved by agent_parser in real-time)
			var existingAgentMsgID string
			_ = DB.QueryRow(`
				SELECT id FROM messages 
				WHERE conversation_id = ? AND role = 'agent' 
				  AND (content = ? OR (length(content) > 30 AND length(?) > 30 AND substr(content, 1, 30) = substr(?, 1, 30)) OR content = '')
				  AND ABS(strftime('%s', created_at) - ?) < 300
				LIMIT 1
			`, convID, currentAgentContent, currentAgentContent, currentAgentContent, currentAgentTime/1000).Scan(&existingAgentMsgID)

			if existingAgentMsgID == "" {
				_ = SaveAgentMessage(currentAgentMsgID, convID, currentAgentContent, currentAgentThinking, currentAgentImg, currentAgentTCs, currentAgentTime)
			}
			currentAgentMsgID = ""
			currentAgentContent = ""
			currentAgentThinking = nil
			currentAgentImg = nil
			currentAgentTCs = nil
		}
	}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item map[string]interface{}
		if err := json.Unmarshal(line, &item); err != nil {
			continue
		}

		timestamp := time.Now().UnixMilli()
		if caStr, ok := item["created_at"].(string); ok {
			if t, err := time.Parse(time.RFC3339, caStr); err == nil {
				timestamp = t.UnixMilli()
			}
		}

		source, _ := item["source"].(string)
		itemType, _ := item["type"].(string)

		if source == "USER_EXPLICIT" && itemType == "USER_INPUT" {
			flushAgent()

			text, _ := item["content"].(string)
			var imageUrls []string

			match := userReqRegex.FindStringSubmatch(text)
			if len(match) > 1 {
				text = strings.TrimSpace(match[1])
			} else {
				text = strings.TrimSpace(tagRegex.ReplaceAllString(text, ""))
			}

			matches := photoTagRegex.FindAllStringSubmatch(text, -1)
			for _, m := range matches {
				if len(m) > 1 {
					p := strings.TrimSpace(m[1])
					if strings.Contains(p, "/uploads/") {
						imageUrls = append(imageUrls, "/uploads/"+filepath.Base(p))
					} else {
						imageUrls = append(imageUrls, "/api/media?path="+url.QueryEscape(p))
					}
				}
			}
			text = strings.TrimSpace(photoTagRegex.ReplaceAllString(text, ""))

			var firstImage string
			if len(imageUrls) > 0 {
				firstImage = imageUrls[0]
			}

			stepIdx := 0
			if s, ok := item["step_index"].(float64); ok {
				stepIdx = int(s)
			}
			msgID := fmt.Sprintf("msg_%d", stepIdx)

			// Check if this user message already exists in DB (e.g. saved by agent_runner with client-generated ID)
			var existingUserMsgID string
			_ = DB.QueryRow(`
				SELECT id FROM messages 
				WHERE conversation_id = ? AND role = 'user' AND content = ? 
				  AND ABS(strftime('%s', created_at) - ?) < 300
				LIMIT 1
			`, convID, text, timestamp/1000).Scan(&existingUserMsgID)

			if existingUserMsgID == "" {
				_ = SaveUserMessage(msgID, convID, text, firstImage, imageUrls, timestamp)
			}

		} else if source == "MODEL" && itemType == "PLANNER_RESPONSE" {
			stepIdx := 0
			if s, ok := item["step_index"].(float64); ok {
				stepIdx = int(s)
			}
			msgID := fmt.Sprintf("msg_%d", stepIdx)

			if currentAgentMsgID == "" {
				currentAgentMsgID = msgID
				currentAgentTime = timestamp
			}

			if tcList, ok := item["tool_calls"].([]interface{}); ok {
				for i, tc := range tcList {
					if tcMap, ok := tc.(map[string]interface{}); ok {
						name, _ := tcMap["name"].(string)
						rawArgs, _ := tcMap["args"].(map[string]interface{})
						cleanArgs := make(map[string]interface{})
						for k, v := range rawArgs {
							if strVal, ok := v.(string); ok {
								cleanArgs[k] = strings.Trim(strings.TrimSpace(strVal), " \t\r\n\"'`")
							} else {
								cleanArgs[k] = v
							}
						}
						currentAgentTCs = append(currentAgentTCs, ToolCallRecord{
							ID:      fmt.Sprintf("tool_%d_%d", stepIdx, i),
							Name:    name,
							Summary: FormatToolSummary(name, cleanArgs),
							Args:    cleanArgs,
							Status:  "completed",
						})
					}
				}
			}

			content, _ := item["content"].(string)
			if content != "" {
				if currentAgentContent != "" {
					currentAgentContent += "\n\n" + content
				} else {
					currentAgentContent = content
				}
			}

			if th, ok := item["thinking"].(string); ok && strings.TrimSpace(th) != "" {
				trimmed := strings.TrimSpace(th)
				if currentAgentThinking != nil {
					combined := *currentAgentThinking + "\n\n" + trimmed
					currentAgentThinking = &combined
				} else {
					currentAgentThinking = &trimmed
				}
			}
		}
	}
	flushAgent()
}
