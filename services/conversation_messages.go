package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
)

type ToolCall struct {
	ID      string                 `json:"id"`
	Name    string                 `json:"name"`
	Summary string                 `json:"summary"`
	Args    map[string]interface{} `json:"args"`
	Output  string                 `json:"output,omitempty"`
	Status  string                 `json:"status"`
}

type ChatMessage struct {
	ID        string     `json:"id"`
	Role      string     `json:"role"`
	Content   string     `json:"content"`
	Thinking  *string    `json:"thinking"`
	Timestamp int64      `json:"timestamp"`
	ImageURL  *string    `json:"imageUrl"`
	ImageURLs []string   `json:"imageUrls"`
	ToolCalls []ToolCall `json:"toolCalls"`
}

func FormatToolSummary(name string, args map[string]interface{}) string {
	return db.FormatToolSummary(name, args)
}

func mapDBMessagesToChatMessages(records []db.MessageRecord) []ChatMessage {
	var messages []ChatMessage
	for _, m := range records {
		var tcs []ToolCall
		for _, tc := range m.ToolCalls {
			tcs = append(tcs, ToolCall{
				ID:      tc.ID,
				Name:    tc.Name,
				Summary: tc.Summary,
				Args:    tc.Args,
				Output:  tc.Output,
				Status:  tc.Status,
			})
		}
		if tcs == nil {
			tcs = []ToolCall{}
		}
		imgUrls := m.ImageURLs
		if imgUrls == nil {
			imgUrls = []string{}
		}
		messages = append(messages, ChatMessage{
			ID:        m.ID,
			Role:      m.Role,
			Content:   m.Content,
			Thinking:  m.Thinking,
			Timestamp: m.Timestamp,
			ImageURL:  m.ImageURL,
			ImageURLs: imgUrls,
			ToolCalls: tcs,
		})
	}
	return messages
}

func LoadConversationMessages(convID string, limit int) []ChatMessage {
	transcriptPath := filepath.Join(config.AppConfig.BrainDir, convID, ".system_generated/logs/transcript.jsonl")
	if fi, err := os.Stat(transcriptPath); err == nil {
		lastDBCa := db.GetLastMessageTime(convID)
		if lastDBCa.IsZero() || fi.ModTime().After(lastDBCa.Add(2*time.Second)) {
			db.ImportSingleConversationTranscript(convID)
		}
	}

	// 1. Try SQLite first (ultra fast <2ms)
	dbMsgs, err := db.GetMessagesFromDB(convID, limit)
	if err == nil && len(dbMsgs) > 0 {
		return mapDBMessagesToChatMessages(dbMsgs)
	}

	// 2. If not found in SQLite, import from legacy transcript into SQLite
	db.ImportSingleConversationTranscript(convID)
	dbMsgs, err = db.GetMessagesFromDB(convID, limit)
	if err == nil && len(dbMsgs) > 0 {
		return mapDBMessagesToChatMessages(dbMsgs)
	}

	// 3. Fallback to parsing transcript directly if SQLite failed
	transcriptPath = filepath.Join(config.AppConfig.BrainDir, convID, ".system_generated/logs/transcript.jsonl")
	file, err := os.Open(transcriptPath)
	if err != nil {
		return []ChatMessage{}
	}
	defer file.Close()

	var messages []ChatMessage
	var currentAgentMsg *ChatMessage

	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

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
			text, _ := item["content"].(string)
			var imageUrls []string

			match := userReqRegex.FindStringSubmatch(text)
			if len(match) > 1 {
				text = strings.TrimSpace(match[1])
			} else {
				text = strings.TrimSpace(tagRegex.ReplaceAllString(text, ""))
			}

			// Extract photo tags
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

			// Extract media items
			if mediaList, ok := item["media"].([]interface{}); ok {
				for _, m := range mediaList {
					if mObj, ok := m.(map[string]interface{}); ok {
						if uri, ok := mObj["uri"].(string); ok {
							clean := strings.TrimPrefix(uri, "file://")
							imageUrls = append(imageUrls, "/api/media?path="+url.QueryEscape(clean))
						}
					}
				}
			}

			var firstImage *string
			if len(imageUrls) > 0 {
				firstImage = &imageUrls[0]
			}

			stepIdx := 0
			if s, ok := item["step_index"].(float64); ok {
				stepIdx = int(s)
			}

			messages = append(messages, ChatMessage{
				ID:        fmt.Sprintf("msg_%d", stepIdx),
				Role:      "user",
				Content:   text,
				Timestamp: timestamp,
				ImageURL:  firstImage,
				ImageURLs: imageUrls,
				ToolCalls: []ToolCall{},
			})
			currentAgentMsg = nil

		} else if source == "MODEL" && itemType == "PLANNER_RESPONSE" {
			var toolCalls []ToolCall
			if tcList, ok := item["tool_calls"].([]interface{}); ok {
				for i, tc := range tcList {
					if tcMap, ok := tc.(map[string]interface{}); ok {
						name, _ := tcMap["name"].(string)
						args, _ := tcMap["args"].(map[string]interface{})
						stepIdx := 0
						if s, ok := item["step_index"].(float64); ok {
							stepIdx = int(s)
						}
						toolCalls = append(toolCalls, ToolCall{
							ID:      fmt.Sprintf("tool_%d_%d", stepIdx, i),
							Name:    name,
							Summary: FormatToolSummary(name, args),
							Args:    args,
							Status:  "completed",
						})
					}
				}
			}

			agentText, _ := item["content"].(string)
			var thinkingPtr *string
			if th, ok := item["thinking"].(string); ok && strings.TrimSpace(th) != "" {
				trimmed := strings.TrimSpace(th)
				thinkingPtr = &trimmed
			}

			var agentImgURL *string
			if mediaList, ok := item["media"].([]interface{}); ok && len(mediaList) > 0 {
				if mObj, ok := mediaList[0].(map[string]interface{}); ok {
					if uri, ok := mObj["uri"].(string); ok {
						clean := strings.TrimPrefix(uri, "file://")
						formatted := "/api/media?path=" + url.QueryEscape(clean)
						agentImgURL = &formatted
					}
				}
			}

			if currentAgentMsg == nil {
				stepIdx := 0
				if s, ok := item["step_index"].(float64); ok {
					stepIdx = int(s)
				}
				newMsg := ChatMessage{
					ID:        fmt.Sprintf("msg_%d", stepIdx),
					Role:      "agent",
					Content:   agentText,
					Thinking:  thinkingPtr,
					Timestamp: timestamp,
					ImageURL:  agentImgURL,
					ToolCalls: toolCalls,
				}
				messages = append(messages, newMsg)
				currentAgentMsg = &messages[len(messages)-1]
			} else {
				if thinkingPtr != nil {
					if currentAgentMsg.Thinking != nil {
						combined := *currentAgentMsg.Thinking + "\n\n" + *thinkingPtr
						currentAgentMsg.Thinking = &combined
					} else {
						currentAgentMsg.Thinking = thinkingPtr
					}
				}
				if agentText != "" {
					if currentAgentMsg.Content != "" {
						currentAgentMsg.Content += "\n\n" + agentText
					} else {
						currentAgentMsg.Content = agentText
					}
				}
				if agentImgURL != nil && currentAgentMsg.ImageURL == nil {
					currentAgentMsg.ImageURL = agentImgURL
				}
				if len(toolCalls) > 0 {
					currentAgentMsg.ToolCalls = append(currentAgentMsg.ToolCalls, toolCalls...)
				}
			}

		} else if itemType == "GENERIC" && currentAgentMsg != nil {
			if mediaList, ok := item["media"].([]interface{}); ok && len(mediaList) > 0 && currentAgentMsg.ImageURL == nil {
				if mObj, ok := mediaList[0].(map[string]interface{}); ok {
					if uri, ok := mObj["uri"].(string); ok {
						clean := strings.TrimPrefix(uri, "file://")
						formatted := "/api/media?path=" + url.QueryEscape(clean)
						currentAgentMsg.ImageURL = &formatted
					}
				}
			}
			if len(currentAgentMsg.ToolCalls) > 0 {
				lastIdx := len(currentAgentMsg.ToolCalls) - 1
				if currentAgentMsg.ToolCalls[lastIdx].Output == "" {
					if c, ok := item["content"].(string); ok {
						currentAgentMsg.ToolCalls[lastIdx].Output = c
					}
				}
			}
		}
	}

	if limit > 0 && len(messages) > limit {
		return messages[len(messages)-limit:]
	}
	return messages
}
