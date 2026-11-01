package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"antigravity-bridge/config"
	"antigravity-bridge/db"
)

func validateCwd(targetCwd string) string {
	defaultWs, _ := filepath.Abs(config.AppConfig.DefaultWorkspace)
	if targetCwd == "" {
		return defaultWs
	}
	abs, err := filepath.Abs(targetCwd)
	if err != nil {
		return defaultWs
	}
	if defaultWs != "" && defaultWs != "/" {
		if abs != defaultWs && !strings.HasPrefix(abs, defaultWs+string(filepath.Separator)) {
			return defaultWs
		}
	}
	if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
		return abs
	}
	return defaultWs
}

func (s *Session) ExecuteTurn(task AgentTask) {
	defer func() {
		if rec := recover(); rec != nil {
			stackBuf := make([]byte, 4096)
			n := runtime.Stack(stackBuf, false)
			stackTrace := string(stackBuf[:n])
			errStr := fmt.Sprintf("%v", rec)

			fmt.Printf("[Agent Turn Panic Recovered] Session %s: %s\n%s\n", s.ID, errStr, stackTrace)

			ReportCrash("AGENT_EXECUTION_PANIC", errStr, stackTrace, map[string]interface{}{
				"sessionID": s.ID,
				"prompt":    task.Prompt,
			})

			s.Lock()
			s.Status = "idle"
			s.Unlock()
			s.Broadcast(map[string]interface{}{
				"type":  "error",
				"error": "Erro inesperado ao executar agente. Recuperado com segurança.",
			})
		}
	}()

	s.Lock()
	s.Status = "running"
	s.History = make([]interface{}, 0)
	s.Unlock()

	s.Broadcast(map[string]interface{}{
		"type":           "turn_started",
		"messageId":      task.MessageID,
		"conversationId": s.ID,
		"prompt":         task.Prompt,
		"imageUrl":       task.ImageURL,
		"imageUrls":      task.ImageURLs,
	})

	args := []string{
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--mode", "auto",
		"--dangerously-skip-permissions",
	}

	isNewSession := s.ID == "" || s.ID == "default" || strings.HasPrefix(s.ID, "new_") || strings.HasPrefix(s.ID, "temp_")
	if !isNewSession {
		args = append(args, "--conversation", s.ID)
		_ = db.UpsertConversation(s.ID, task.GetOwner(), task.Prompt, time.Now())
		_ = db.SaveUserMessage(task.MessageID, s.ID, task.Prompt, task.ImageURL, task.ImageURLs, task.Timestamp)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cwd := validateCwd(task.Cwd)

	fmt.Printf("[WS] Spawning turn: %s %s (cwd: %s)\n", config.AppConfig.AgyBin, strings.Join(args, " "), cwd)

	cmd := exec.CommandContext(ctx, config.AppConfig.AgyBin, args...)
	cmd.Dir = cwd
	prepareProcess(cmd)

	home, _ := os.UserHomeDir()
	geminiBin := filepath.Join(home, ".gemini", "antigravity-cli", "bin")
	localBin := filepath.Join(home, ".local", "bin")
	pathEnv := strings.Join([]string{geminiBin, localBin, os.Getenv("PATH")}, string(os.PathListSeparator))
	cmd.Env = append(os.Environ(),
		"PATH="+pathEnv,
		"PYTHONUNBUFFERED=1",
		"NODE_NO_WARNINGS=1",
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		s.Broadcast(map[string]interface{}{
			"type":  "error",
			"error": fmt.Sprintf("Failed to open stdin: %v", err),
		})
		return
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		s.Broadcast(map[string]interface{}{
			"type":  "error",
			"error": fmt.Sprintf("Failed to open stdout: %v", err),
		})
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		s.Broadcast(map[string]interface{}{
			"type":  "error",
			"error": fmt.Sprintf("Failed to open stderr: %v", err),
		})
		return
	}

	if err := cmd.Start(); err != nil {
		cancel()
		ReportError("AGENT_SPAWN_FAILED", err, map[string]interface{}{
			"sessionID": s.ID,
			"agyBin":    config.AppConfig.AgyBin,
			"cwd":       cwd,
		})
		s.Broadcast(map[string]interface{}{
			"type":  "error",
			"error": fmt.Sprintf("Failed to spawn agy: %v", err),
		})
		return
	}

	s.Lock()
	s.Cmd = cmd
	s.Cancel = cancel
	s.Stdin = stdinPipe
	s.Unlock()

	var once sync.Once
	checkNextInQueue := func() {
		once.Do(func() {
			s.Lock()
			s.Cmd = nil
			s.Stdin = nil
			if s.Cancel != nil {
				s.Cancel()
				s.Cancel = nil
			}

			if len(s.Queue) > 0 {
				nextTask := s.Queue[0]
				s.Queue = s.Queue[1:]
				s.Unlock()

				fmt.Printf("[WS] ⏩ Processing next queued task for session %s (%d remaining)\n", s.ID, len(s.Queue))
				go func() {
					time.Sleep(100 * time.Millisecond)
					s.ExecuteTurn(nextTask)
				}()
			} else {
				s.Status = "idle"
				s.History = make([]interface{}, 0)
				s.Unlock()
				s.Broadcast(map[string]interface{}{
					"type": "idle",
				})
			}
		})
	}

	// Read stdout in background goroutine
	go s.processStdout(stdoutPipe, stdinPipe, cmd, task, checkNextInQueue)

	// Read stderr in background goroutine
	go s.processStderr(stderrPipe)

	// Send initial prompt to agy stdin
	payloadBytes := buildUserPromptPayload(task)
	_, _ = stdinPipe.Write(payloadBytes)
}
