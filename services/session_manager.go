package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"antigravity-bridge/config"
	"github.com/gorilla/websocket"
)

func getSessionOwner() string {
	if len(config.AppConfig.AllowedEmails) > 0 && config.AppConfig.AllowedEmails[0] != "*" {
		return config.AppConfig.AllowedEmails[0]
	}
	return "user@antigravity"
}

type AgentTask struct {
	UserEmail  string   `json:"userEmail"`
	MessageID  string   `json:"messageId"`
	Prompt     string   `json:"prompt"`
	ImagePath  string   `json:"imagePath"`
	ImagePaths []string `json:"imagePaths"`
	ImageURL   string   `json:"imageUrl"`
	ImageURLs  []string `json:"imageUrls"`
	Cwd        string   `json:"cwd"`
	Timestamp  int64    `json:"timestamp"`
}

func (t *AgentTask) GetOwner() string {
	if t.UserEmail != "" {
		return t.UserEmail
	}
	return getSessionOwner()
}

type Session struct {
	sync.Mutex
	ID      string
	Cmd     *exec.Cmd
	Cancel  context.CancelFunc
	Stdin   io.WriteCloser
	History []interface{}
	Clients map[*websocket.Conn]bool
	Status  string
	Queue   []AgentTask
}

type SessionManager struct {
	sync.RWMutex
	sessions        map[string]*Session
	latestSessionID string
}

var Manager = &SessionManager{
	sessions: make(map[string]*Session),
}

func InitSessionManager() {
	Manager.Lock()
	defer Manager.Unlock()

	convs := ListConversations()
	if len(convs) > 0 {
		Manager.latestSessionID = convs[0].ID
		fmt.Printf("[WS] Initialized latest active session to: %s\n", Manager.latestSessionID)
	}
}

func (sm *SessionManager) GetLatestSessionID() string {
	sm.RLock()
	defer sm.RUnlock()
	return sm.latestSessionID
}

func (sm *SessionManager) SetLatestSessionID(id string) {
	sm.Lock()
	defer sm.Unlock()
	sm.latestSessionID = id
}

func (sm *SessionManager) BroadcastAll(payload interface{}) {
	sm.RLock()
	defer sm.RUnlock()
	for _, s := range sm.sessions {
		s.Broadcast(payload)
	}
}

func (sm *SessionManager) GetSession(convID string) *Session {
	sm.Lock()
	defer sm.Unlock()

	id := convID
	if id == "" || id == "default" {
		id = fmt.Sprintf("new_%d_%s", time.Now().UnixMilli(), RandomHex(3))
	}

	s, ok := sm.sessions[id]
	if !ok {
		s = &Session{
			ID:      id,
			History: make([]interface{}, 0, 50),
			Clients: make(map[*websocket.Conn]bool),
			Status:  "idle",
			Queue:   make([]AgentTask, 0),
		}
		sm.sessions[id] = s
	}
	return s
}

func (sm *SessionManager) ActiveSessionsCount() int {
	sm.RLock()
	defer sm.RUnlock()
	return len(sm.sessions)
}

func (s *Session) Broadcast(payload interface{}) {
	s.Lock()
	s.History = append(s.History, payload)
	if len(s.History) > 200 {
		s.History = s.History[1:]
	}

	data, err := json.Marshal(payload)
	if err != nil {
		s.Unlock()
		return
	}

	for client := range s.Clients {
		_ = client.WriteMessage(websocket.TextMessage, data)
	}
	s.Unlock()
}

func (s *Session) KillProcess() {
	s.Lock()
	s.Queue = make([]AgentTask, 0)

	if s.Cancel != nil {
		s.Cancel()
		s.Cancel = nil
	}

	if s.Cmd != nil && s.Cmd.Process != nil {
		pid := s.Cmd.Process.Pid
		fmt.Printf("[WS] 🛑 Terminating process group for session %s (PID %d)\n", s.ID, pid)
		// Send SIGTERM to process group
		_ = syscall.Kill(-pid, syscall.SIGTERM)
		time.Sleep(200 * time.Millisecond)
		// Force SIGKILL if still running
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		s.Cmd = nil
		s.Stdin = nil
	}

	s.Status = "idle"
	s.History = make([]interface{}, 0)
	s.Unlock()

	s.Broadcast(map[string]interface{}{
		"type": "stopped",
	})
}
