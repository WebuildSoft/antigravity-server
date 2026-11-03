package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"antigravity-bridge/db"
)

func buildUserPromptPayload(task AgentTask) []byte {
	streamPrompt := task.Prompt
	if len(task.ImagePaths) > 0 {
		var lines []string
		for _, p := range task.ImagePaths {
			lines = append(lines, fmt.Sprintf("[Foto enviada pelo usuário: %s]", p))
		}
		if strings.TrimSpace(streamPrompt) == "" {
			streamPrompt = strings.Join(lines, "\n") + "\nPor favor analise as imagens enviadas."
		} else {
			streamPrompt = strings.Join(lines, "\n") + "\n" + streamPrompt
		}
	} else if task.ImagePath != "" {
		if strings.TrimSpace(streamPrompt) == "" {
			streamPrompt = fmt.Sprintf("[Foto enviada pelo usuário: %s]\nPor favor analise a imagem enviada.", task.ImagePath)
		} else {
			streamPrompt = fmt.Sprintf("[Foto enviada pelo usuário: %s]\n%s", task.ImagePath, streamPrompt)
		}
	}

	payload := map[string]interface{}{
		"event": "user",
		"message": map[string]interface{}{
			"content": streamPrompt,
		},
	}
	data, _ := json.Marshal(payload)
	return append(data, '\n')
}

func (s *Session) processStdout(stdout io.Reader, stdin io.Closer, cmd *exec.Cmd, task AgentTask, checkNextInQueue func()) {
	scanner := bufio.NewScanner(stdout)
	buf := make([]byte, 0, 512*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var accumulatedAgentText strings.Builder
	var accumulatedThinking strings.Builder
	var accumulatedToolCalls []db.ToolCallRecord
	toolIndices := make(map[string]int)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var eventObj map[string]interface{}
		if err := json.Unmarshal(line, &eventObj); err != nil {
			continue
		}

		eventName, _ := eventObj["event"].(string)

		switch eventName {
		case "init":
			newConvID, _ := eventObj["conversation_id"].(string)
			if newConvID != "" && newConvID != s.ID {
				Manager.Lock()
				delete(Manager.sessions, s.ID)
				s.ID = newConvID
				Manager.latestSessionID = newConvID
				Manager.sessions[newConvID] = s
				Manager.Unlock()
			}
			_ = db.UpsertConversation(s.ID, task.GetOwner(), task.Prompt, time.Now())
			_ = db.SaveUserMessage(task.MessageID, s.ID, task.Prompt, task.ImageURL, task.ImageURLs, task.Timestamp)

			initData, _ := eventObj["init"].(map[string]interface{})
			s.Broadcast(map[string]interface{}{
				"type":           "init",
				"conversationId": newConvID,
				"tools":          initData["tools"],
				"cwd":            initData["cwd"],
			})

		case "step_update":
			su, _ := eventObj["step_update"].(map[string]interface{})
			stepType, _ := su["step_type"].(string)
			state, _ := su["state"].(string)
			stepIndex, _ := su["step_index"].(float64)
			textDelta, _ := su["text_delta"].(string)
			thinkingDelta, _ := su["thinking_delta"].(string)
			if thinkingDelta == "" {
				thinkingDelta, _ = su["thinking"].(string)
			}

			if textDelta != "" {
				accumulatedAgentText.WriteString(textDelta)
			}
			if thinkingDelta != "" {
				accumulatedThinking.WriteString(thinkingDelta)
			}

			var toolInfo map[string]interface{}
			toolName, _ := su["tool_name"].(string)
			if stepType == "tool" || toolName != "" {
				tInfoMap, _ := su["tool_info"].(map[string]interface{})
				name := toolName
				if name == "" && tInfoMap != nil {
					name, _ = tInfoMap["name"].(string)
				}
				if name == "" {
					name = "tool"
				}
				var params map[string]interface{}
				if tInfoMap != nil {
					params, _ = tInfoMap["parameters"].(map[string]interface{})
				}
				output, _ := tInfoMap["output"].(string)
				toolStatus := "running"
				if state == "DONE" {
					toolStatus = "completed"
				}

				tcID := fmt.Sprintf("tool_%d", int(stepIndex))
				summary := FormatToolSummary(name, params)

				toolInfo = map[string]interface{}{
					"id":      tcID,
					"name":    name,
					"summary": summary,
					"args":    params,
					"output":  output,
					"status":  toolStatus,
				}

				if idx, found := toolIndices[tcID]; found {
					if output != "" {
						accumulatedToolCalls[idx].Output = output
					}
					if toolStatus != "" {
						accumulatedToolCalls[idx].Status = toolStatus
					}
				} else {
					toolIndices[tcID] = len(accumulatedToolCalls)
					accumulatedToolCalls = append(accumulatedToolCalls, db.ToolCallRecord{
						ID:      tcID,
						Name:    name,
						Summary: summary,
						Args:    params,
						Output:  output,
						Status:  toolStatus,
					})
				}
			}

			s.Broadcast(map[string]interface{}{
				"type":          "step_update",
				"stepIndex":     int(stepIndex),
				"stepType":      stepType,
				"state":         state,
				"textDelta":     textDelta,
				"thinkingDelta": thinkingDelta,
				"toolInfo":      toolInfo,
				"raw":           su,
			})

		case "result":
			resMap, _ := eventObj["result"].(map[string]interface{})
			resConvID, _ := resMap["conversation_id"].(string)
			if resConvID == "" {
				resConvID = s.ID
			}
			finalText, _ := resMap["response"].(string)
			if finalText == "" {
				finalText = accumulatedAgentText.String()
			}
			var thinkingPtr *string
			if accumulatedThinking.Len() > 0 {
				th := accumulatedThinking.String()
				thinkingPtr = &th
			}
			nowMs := time.Now().UnixMilli()
			agentMsgID := fmt.Sprintf("agent_%d", nowMs)
			_ = db.SaveAgentMessage(agentMsgID, resConvID, finalText, thinkingPtr, nil, accumulatedToolCalls, nowMs)
			_ = db.UpsertConversation(resConvID, task.GetOwner(), task.Prompt, time.Now())

			s.Broadcast(map[string]interface{}{
				"type":            "result",
				"messageId":       task.MessageID,
				"conversationId":  resConvID,
				"status":          resMap["status"],
				"response":        resMap["response"],
				"durationSeconds": resMap["duration_seconds"],
				"usage":           resMap["usage"],
			})

			if stdin != nil {
				_ = stdin.Close()
			}
			go func() {
				time.Sleep(200 * time.Millisecond)
				terminateProcess(cmd)
				checkNextInQueue()
			}()

		default:
			s.Broadcast(map[string]interface{}{
				"type": "agy_event",
				"data": eventObj,
			})
		}
	}

	waitErr := cmd.Wait()

	s.Lock()
	stderrLines := make([]string, len(s.StderrBuffer))
	copy(stderrLines, s.StderrBuffer)
	s.Unlock()

	hasOutput := accumulatedAgentText.Len() > 0 || len(accumulatedToolCalls) > 0
	if !hasOutput || waitErr != nil {
		stderrCombined := strings.Join(stderrLines, "\n")
		friendlyErr := formatFriendlyErrorMessage(waitErr, stderrCombined)
		if friendlyErr != "" {
			fmt.Printf("[WS] ⚠️ Detected error for session %s: %s\n", s.ID, friendlyErr)

			// 1. Envia erro estruturado para o Flutter exibir alerta
			s.Broadcast(map[string]interface{}{
				"type":    "error",
				"message": friendlyErr,
			})

			// 2. Envia step_update para a bolha do chat exibir a mensagem formatada em Markdown
			s.Broadcast(map[string]interface{}{
				"type": "step_update",
				"step_update": map[string]interface{}{
					"conversation_id": s.ID,
					"step_index":      1,
					"state":           "DONE",
					"step_type":       "agent_response",
					"text_delta":      friendlyErr,
				},
			})

			// 3. Salva no histórico da conversa no banco SQLite
			_ = db.SaveAgentMessage(task.MessageID+"_err", s.ID, friendlyErr, nil, nil, nil, time.Now().UnixMilli())
		}
	}

	checkNextInQueue()
}

func formatFriendlyErrorMessage(cmdErr error, stderrOutput string) string {
	lowerStderr := strings.ToLower(stderrOutput)

	if strings.Contains(lowerStderr, "google tos") || strings.Contains(lowerStderr, "terms of service") || strings.Contains(lowerStderr, "not eligible for gemini code assist") {
		return "⚠️ **Termos de Serviço Pendentes ou Conta Não Elegível (Google TOS)**\n\n" +
			"A conta Google ativa não concluiu o aceite dos Termos de Serviço do **Gemini Code Assist for Individuals**.\n\n" +
			"**Como Resolver:**\n" +
			"1. Acesse **[codeassist.google](https://codeassist.google/)** no navegador conectado a esta conta e aceite os termos;\n" +
			"2. Ou abra o Terminal no Mac e execute `~/.local/bin/agy` uma vez para aceitar o prompt;\n" +
			"3. Ou no app, vá em **Gerenciar Contas** e alterne para a sua conta principal autorizada."
	}

	if strings.Contains(lowerStderr, "unauthenticated") || strings.Contains(lowerStderr, "code 401") || strings.Contains(lowerStderr, "invalid authentication credentials") {
		return "🔒 **Erro de Autenticação (Código 401)**\n\n" +
			"As credenciais da conta Google ativa expiraram, foram revogadas ou não foram encontradas.\n\n" +
			"**Como Resolver:**\n" +
			"1. No aplicativo ou web, abra o menu lateral e acesse **Gerenciar Contas**;\n" +
			"2. Clique na sua conta para reativá-la ou faça login novamente;\n" +
			"3. Envie sua mensagem novamente."
	}

	if strings.Contains(lowerStderr, "exhausted") || strings.Contains(lowerStderr, "quota") || strings.Contains(lowerStderr, "429") {
		return "⏳ **Limite de Cota Atingido**\n\n" +
			"A cota de uso do modelo nesta conta atingiu o limite temporário.\n\n" +
			"**Como Resolver:**\n" +
			"1. Alterne para outra conta Google no menu lateral para continuar conversando;\n" +
			"2. Ou aguarde a renovação da cota."
	}

	if cmdErr != nil || strings.TrimSpace(stderrOutput) != "" {
		trimmed := strings.TrimSpace(stderrOutput)
		if len(trimmed) > 300 {
			trimmed = trimmed[:300] + "..."
		}
		if trimmed == "" && cmdErr != nil {
			trimmed = cmdErr.Error()
		}
		return fmt.Sprintf("⚠️ **Falha ao Executar Assistente**\n\n%s\n\nTente alternar de conta ou enviar a mensagem novamente.", trimmed)
	}

	return ""
}

func (s *Session) processStderr(stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		text := scanner.Text()
		fmt.Printf("[agy stderr]: %s\n", text)
		s.Lock()
		s.StderrBuffer = append(s.StderrBuffer, text)
		if len(s.StderrBuffer) > 50 {
			s.StderrBuffer = s.StderrBuffer[len(s.StderrBuffer)-50:]
		}
		s.Unlock()
		s.Broadcast(map[string]interface{}{
			"type": "stderr",
			"text": text,
		})
	}
}
