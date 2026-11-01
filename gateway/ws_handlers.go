package gateway

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"antigravity-bridge/auth"
	"antigravity-bridge/config"
	"antigravity-bridge/db"
	"antigravity-bridge/services"
	"github.com/gorilla/websocket"
)

func (c *ClientConn) dispatchAction(action string, data map[string]interface{}, isRemote bool) {
	defer func() {
		if rec := recover(); rec != nil {
			stackBuf := make([]byte, 4096)
			n := runtime.Stack(stackBuf, false)
			stackTrace := string(stackBuf[:n])
			errStr := fmt.Sprintf("%v", rec)

			fmt.Printf("[WS Panic Recovered] Action '%s': %s\n%s\n", action, errStr, stackTrace)

			services.ReportCrash("WS_ACTION_PANIC", errStr, stackTrace, map[string]interface{}{
				"action":   action,
				"isRemote": isRemote,
			})

			_ = c.Send(map[string]interface{}{
				"type":    "error",
				"message": "Erro inesperado ao processar comando. Foi registrado com segurança.",
			})
		}
	}()

	switch action {
	case "auth":
		c.handleAuth(data)
	case "ping":
		c.handlePing()
	case "get_conversations":
		c.handleGetConversations()
	case "get_quota":
		c.handleGetQuota(data)
	case "get_accounts", "list_accounts", "switch_account", "import_account", "exchange_google_code", "remove_account":
		c.handleAccounts(action, data, isRemote)
	case "reset_conversation", "new_conversation":
		c.handleResetConversation()
	case "list_files", "read_file", "save_file":
		c.handleFiles(action, data, isRemote)
	case "load_conversation":
		c.handleLoadConversation(data)
	case "load_all_messages":
		c.handleLoadAllMessages(data)
	case "attach":
		c.handleAttach(data)
	case "stop":
		c.handleStop(data)
	case "send":
		c.handleSend(data, isRemote)
	}
}

func (c *ClientConn) handleAuth(data map[string]interface{}) {
	tok, _ := data["token"].(string)
	user, err := auth.VerifyFirebaseToken(tok)
	if err == nil && user != nil {
		if !config.AppConfig.IsEmailAllowed(user.Email) {
			fmt.Printf("[Firebase Auth] ⛔ Acesso BLOQUEADO para e-mail não autorizado: %s\n", user.Email)
			_ = c.Send(map[string]interface{}{
				"type":    "auth_error",
				"code":    "unauthorized_user",
				"message": "Acesso restrito. Este usuário não tem autorização para utilizar este sistema.",
			})
			_ = c.ws.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(4003, "Unauthorized User"),
				time.Now().Add(time.Second))
			_ = c.ws.Close()
			return
		}
		c.authUser = user
		fmt.Printf("[Firebase Auth] Usuário autenticado e autorizado: %s (%s)\n", user.Email, user.UID)
		_ = c.Send(map[string]interface{}{
			"type":  "auth_success",
			"email": user.Email,
			"uid":   user.UID,
		})
	} else {
		_ = c.Send(map[string]interface{}{
			"type":    "auth_error",
			"message": "Token de autenticação inválido.",
		})
	}
}

func (c *ClientConn) handlePing() {
	c.Lock()
	c.isAlive = true
	c.Unlock()
	_ = c.Send(map[string]interface{}{
		"type":      "pong",
		"timestamp": time.Now().UnixMilli(),
	})
}

func (c *ClientConn) handleGetConversations() {
	uEmail := ""
	if c.authUser != nil {
		uEmail = c.authUser.Email
	}
	_ = c.Send(map[string]interface{}{
		"type":          "conversations_list",
		"conversations": services.ListConversationsForUser(uEmail),
	})
}

func (c *ClientConn) handleGetQuota(data map[string]interface{}) {
	force, _ := data["refresh"].(bool)
	qData, err := services.GetQuotaData(force)
	if err != nil {
		_ = c.Send(map[string]interface{}{"type": "quota_error", "error": err.Error()})
	} else {
		_ = c.Send(map[string]interface{}{
			"type":  "quota_data",
			"quota": qData,
		})
	}
}

func (c *ClientConn) handleAccounts(action string, data map[string]interface{}, isRemote bool) {
	switch action {
	case "get_accounts", "list_accounts":
		var accounts []services.AccountProfile
		if !isRemote {
			accounts, _ = services.ListAccounts()
		} else if c.authUser != nil && c.authUser.Email != "" {
			accounts, _ = services.ListAccounts()
		}
		if accounts == nil {
			accounts = []services.AccountProfile{}
		}
		_ = c.Send(map[string]interface{}{
			"type":     "accounts_list",
			"accounts": accounts,
		})

	case "switch_account":
		if c.authUser == nil && isRemote {
			_ = c.Send(map[string]interface{}{"type": "account_error", "error": "Autenticação obrigatória."})
			return
		}
		email, _ := data["email"].(string)
		profile, err := services.SwitchAccount(email)
		if err != nil {
			_ = c.Send(map[string]interface{}{
				"type":    "account_error",
				"error":   err.Error(),
				"message": fmt.Sprintf("Erro ao alternar para a conta %s: %v", email, err),
			})
		} else {
			accounts, _ := services.ListAccounts()
			services.Manager.BroadcastAll(map[string]interface{}{
				"type":          "account_switched",
				"activeAccount": profile,
				"accounts":      accounts,
				"message":       fmt.Sprintf("Conta alternada para %s", profile.Email),
			})
		}

	case "import_account":
		if c.authUser == nil && isRemote {
			_ = c.Send(map[string]interface{}{"type": "account_error", "error": "Autenticação obrigatória."})
			return
		}
		rawCredsStr, _ := data["data"].(string)
		if rawCredsStr == "" {
			if d, ok := data["creds"]; ok {
				b, _ := json.Marshal(d)
				rawCredsStr = string(b)
			}
		}
		profile, err := services.ImportAccount([]byte(rawCredsStr))
		if err != nil {
			_ = c.Send(map[string]interface{}{
				"type":  "account_error",
				"error": err.Error(),
			})
		} else {
			accounts, _ := services.ListAccounts()
			services.Manager.BroadcastAll(map[string]interface{}{
				"type":          "account_switched",
				"activeAccount": profile,
				"accounts":      accounts,
				"message":       fmt.Sprintf("Conta %s importada e ativada com sucesso!", profile.Email),
			})
		}

	case "exchange_google_code":
		if c.authUser == nil && isRemote {
			_ = c.Send(map[string]interface{}{"type": "account_error", "error": "Autenticação obrigatória."})
			return
		}
		code, _ := data["code"].(string)
		profile, err := services.ExchangeGoogleAuthCode(code)
		if err != nil {
			_ = c.Send(map[string]interface{}{
				"type":  "account_error",
				"error": err.Error(),
			})
		} else {
			accounts, _ := services.ListAccounts()
			services.Manager.BroadcastAll(map[string]interface{}{
				"type":          "account_switched",
				"activeAccount": profile,
				"accounts":      accounts,
				"message":       fmt.Sprintf("Conta %s conectada com sucesso!", profile.Email),
			})
		}

	case "remove_account":
		if c.authUser == nil && isRemote {
			_ = c.Send(map[string]interface{}{"type": "account_error", "error": "Autenticação obrigatória."})
			return
		}
		email, _ := data["email"].(string)
		err := services.RemoveAccount(email)
		if err != nil {
			_ = c.Send(map[string]interface{}{
				"type":  "account_error",
				"error": err.Error(),
			})
		} else {
			accounts, _ := services.ListAccounts()
			services.Manager.BroadcastAll(map[string]interface{}{
				"type":     "accounts_list",
				"accounts": accounts,
				"message":  fmt.Sprintf("Conta %s removida com sucesso.", email),
			})
		}
	}
}

func (c *ClientConn) handleResetConversation() {
	if c.session != nil {
		c.session.Lock()
		delete(c.session.Clients, c.ws)
		c.session.Unlock()
		c.session = nil
	}
	fmt.Println("[WS] Client detached for new conversation.")
	_ = c.Send(map[string]interface{}{
		"type":    "conversation_reset",
		"status":  "idle",
		"message": "Nova conversa pronta",
	})
}

func (c *ClientConn) handleFiles(action string, data map[string]interface{}, isRemote bool) {
	if c.authUser == nil && isRemote {
		_ = c.Send(map[string]interface{}{"type": "file_error", "error": "Acesso a arquivos restrito."})
		return
	}
	wsPath, _ := data["workspace"].(string)
	if wsPath == "" || wsPath == "~" || wsPath == "." {
		wsPath = config.AppConfig.DefaultWorkspace
	}
	p, _ := data["path"].(string)
	if p == "~" || p == "." {
		p = ""
	}

	switch action {
	case "list_files":
		res, err := services.ListDirectory(p, wsPath)
		if err != nil {
			_ = c.Send(map[string]interface{}{"type": "file_error", "error": err.Error()})
		} else {
			_ = c.Send(map[string]interface{}{
				"type":        "files_listed",
				"currentPath": res.CurrentPath,
				"workspace":   res.Workspace,
				"items":       res.Items,
			})
		}

	case "read_file":
		res, err := services.ReadFileContent(p, wsPath)
		if err != nil {
			_ = c.Send(map[string]interface{}{"type": "file_error", "error": err.Error()})
		} else {
			_ = c.Send(map[string]interface{}{
				"type":        "file_content",
				"path":        res.Path,
				"content":     res.Content,
				"size":        res.Size,
				"modified":    res.Modified,
				"isEditable":  res.IsEditable,
				"currentPath": res.CurrentPath,
			})
		}

	case "save_file":
		cnt, _ := data["content"].(string)
		res, err := services.SaveFileContent(p, cnt, wsPath)
		if err != nil {
			_ = c.Send(map[string]interface{}{"type": "file_error", "error": err.Error()})
		} else {
			_ = c.Send(map[string]interface{}{
				"type":        "file_saved",
				"path":        res.Path,
				"content":     res.Content,
				"size":        res.Size,
				"modified":    res.Modified,
				"isEditable":  res.IsEditable,
				"currentPath": res.CurrentPath,
			})
		}
	}
}

func (c *ClientConn) handleLoadConversation(data map[string]interface{}) {
	targetID, _ := data["conversationId"].(string)
	if targetID == "" {
		return
	}

	if c.authUser != nil {
		if owner, err := db.GetConversationOwner(targetID); err == nil && owner != "" {
			if owner != c.authUser.Email && owner != "user@antigravity" && owner != "*" {
				_ = c.Send(map[string]interface{}{
					"type":    "error",
					"message": "Acesso não autorizado a esta conversa.",
				})
				return
			}
		}
	}

	limit := 35
	if l, ok := data["limit"].(float64); ok && l >= 0 {
		limit = int(l)
	}

	fmt.Printf("[WS] Loading conversation history for %s (limit: %d)\n", targetID, limit)
	messages := services.LoadConversationMessages(targetID, limit)

	if c.session != nil {
		c.session.Lock()
		delete(c.session.Clients, c.ws)
		c.session.Unlock()
	}

	targetSession := services.Manager.GetSession(targetID)
	services.Manager.SetLatestSessionID(targetID)
	targetSession.Lock()
	targetSession.Clients[c.ws] = true
	status := targetSession.Status
	qCount := len(targetSession.Queue)
	targetSession.Unlock()
	c.session = targetSession

	_ = c.Send(map[string]interface{}{
		"type":           "conversation_loaded",
		"conversationId": targetID,
		"messages":       messages,
		"status":         status,
		"queueCount":     qCount,
		"isFullHistory":  limit == 0,
	})
}

func (c *ClientConn) handleLoadAllMessages(data map[string]interface{}) {
	targetID, _ := data["conversationId"].(string)
	if targetID == "" && c.session != nil {
		targetID = c.session.ID
	}
	if targetID == "" || targetID == "default" {
		return
	}

	if c.authUser != nil {
		if owner, err := db.GetConversationOwner(targetID); err == nil && owner != "" {
			if owner != c.authUser.Email && owner != "user@antigravity" && owner != "*" {
				_ = c.Send(map[string]interface{}{
					"type":    "error",
					"message": "Acesso não autorizado a esta conversa.",
				})
				return
			}
		}
	}

	fmt.Printf("[WS] Loading ALL conversation history for %s\n", targetID)
	messages := services.LoadConversationMessages(targetID, 0)

	status := "idle"
	qCount := 0
	if c.session != nil {
		c.session.Lock()
		status = c.session.Status
		qCount = len(c.session.Queue)
		c.session.Unlock()
	}

	_ = c.Send(map[string]interface{}{
		"type":           "conversation_loaded",
		"conversationId": targetID,
		"messages":       messages,
		"status":         status,
		"queueCount":     qCount,
		"isFullHistory":  true,
	})
}

func (c *ClientConn) handleAttach(data map[string]interface{}) {
	reqID, _ := data["conversationId"].(string)
	if reqID == "" || reqID == "default" {
		reqID = services.Manager.GetLatestSessionID()
	}
	if reqID == "" || reqID == "default" {
		uEmail := ""
		if c.authUser != nil {
			uEmail = c.authUser.Email
		}
		convs := services.ListConversationsForUser(uEmail)
		if len(convs) > 0 {
			reqID = convs[0].ID
		}
	}

	if reqID != "" && reqID != "default" {
		if c.session != nil {
			c.session.Lock()
			delete(c.session.Clients, c.ws)
			c.session.Unlock()
		}

		targetSession := services.Manager.GetSession(reqID)
		services.Manager.SetLatestSessionID(reqID)
		targetSession.Lock()
		targetSession.Clients[c.ws] = true
		status := targetSession.Status
		qCount := len(targetSession.Queue)
		histCopy := make([]interface{}, len(targetSession.History))
		copy(histCopy, targetSession.History)
		targetSession.Unlock()
		c.session = targetSession

		fmt.Printf("[WS] Client attached to session %s (status: %s)\n", reqID, status)

		limit := 35
		if l, ok := data["limit"].(float64); ok && l >= 0 {
			limit = int(l)
		}
		messages := services.LoadConversationMessages(reqID, limit)

		_ = c.Send(map[string]interface{}{
			"type":           "conversation_loaded",
			"conversationId": reqID,
			"messages":       messages,
			"status":         status,
			"queueCount":     qCount,
			"isFullHistory":  limit == 0,
		})

		if status == "running" && len(histCopy) > 0 {
			_ = c.Send(map[string]interface{}{
				"type":           "sync",
				"conversationId": reqID,
				"status":         status,
				"history":        histCopy,
			})
		}
	}
}

func (c *ClientConn) handleStop(data map[string]interface{}) {
	targetConvID, _ := data["conversationId"].(string)
	var targetSession *services.Session

	if targetConvID != "" {
		targetSession = services.Manager.GetSession(targetConvID)
	} else if c.session != nil {
		targetSession = c.session
	}

	if targetSession != nil {
		fmt.Printf("[WS] Stop requested for session %s. Terminating process.\n", targetSession.ID)
		targetSession.KillProcess()
	}
}

func (c *ClientConn) handleSend(data map[string]interface{}, isRemote bool) {
	if c.authUser == nil && isRemote {
		_ = c.Send(map[string]interface{}{
			"type":    "error",
			"message": "Autenticação obrigatória. Acesso remoto não autenticado bloqueado.",
		})
		return
	}
	if c.authUser != nil && !config.AppConfig.IsEmailAllowed(c.authUser.Email) {
		_ = c.Send(map[string]interface{}{
			"type":    "error",
			"message": "Acesso restrito. Este usuário não tem autorização para interagir com o agente.",
		})
		return
	}

	userPrompt, _ := data["prompt"].(string)
	if userPrompt == "" {
		userPrompt, _ = data["message"].(string)
	}
	userPrompt = strings.TrimSpace(userPrompt)

	var incomingImages []string
	if imgs, ok := data["images"].([]interface{}); ok {
		for _, img := range imgs {
			if str, ok := img.(string); ok {
				incomingImages = append(incomingImages, str)
			}
		}
	} else if singleImg, ok := data["image"].(string); ok && singleImg != "" {
		incomingImages = append(incomingImages, singleImg)
	} else if b64, ok := data["imageBase64"].(string); ok && b64 != "" {
		incomingImages = append(incomingImages, b64)
	}

	var imagePaths []string
	var imageUrls []string

	for _, b64 := range incomingImages {
		if saved, err := services.SaveBase64Image(b64); err == nil {
			imagePaths = append(imagePaths, saved.Path)
			imageUrls = append(imageUrls, saved.URL)
		}
	}

	if imgPath, ok := data["imagePath"].(string); ok && imgPath != "" {
		imagePaths = append(imagePaths, imgPath)
		imgURL, _ := data["imageUrl"].(string)
		if imgURL == "" {
			imgURL = "/uploads/" + filepath.Base(imgPath)
		}
		imageUrls = append(imageUrls, imgURL)
	}

	if userPrompt == "" && len(imagePaths) == 0 {
		_ = c.Send(map[string]interface{}{"type": "error", "message": "Envie uma mensagem ou foto."})
		return
	}

	cwd, _ := data["cwd"].(string)
	if cwd == "" {
		cwd = config.AppConfig.DefaultWorkspace
	}

	msgID, _ := data["id"].(string)
	if msgID == "" {
		msgID = fmt.Sprintf("msg_%d", time.Now().UnixMilli())
	}

	isExplicitNew := false
	if b, ok := data["isNewConversation"].(bool); ok {
		isExplicitNew = b
	}

	convID, _ := data["conversationId"].(string)
	if isExplicitNew || convID == "" {
		if c.session != nil && !strings.HasPrefix(c.session.ID, "new_") && !isExplicitNew {
			convID = c.session.ID
		} else {
			convID = fmt.Sprintf("new_%d_%s", time.Now().UnixMilli(), services.RandomHex(3))
		}
	}

	if c.session != nil {
		c.session.Lock()
		delete(c.session.Clients, c.ws)
		c.session.Unlock()
	}

	targetSession := services.Manager.GetSession(convID)
	services.Manager.SetLatestSessionID(targetSession.ID)
	targetSession.Lock()
	targetSession.Clients[c.ws] = true
	isBusy := targetSession.Status == "running" && targetSession.Cmd != nil
	targetSession.Unlock()
	c.session = targetSession

	var firstImagePath string
	var firstImageURL string
	if len(imagePaths) > 0 {
		firstImagePath = imagePaths[0]
		firstImageURL = imageUrls[0]
	}

	taskUserEmail := ""
	if c.authUser != nil {
		taskUserEmail = c.authUser.Email
	}

	task := services.AgentTask{
		UserEmail:  taskUserEmail,
		MessageID:  msgID,
		Prompt:     userPrompt,
		ImagePath:  firstImagePath,
		ImagePaths: imagePaths,
		ImageURL:   firstImageURL,
		ImageURLs:  imageUrls,
		Cwd:        cwd,
		Timestamp:  time.Now().UnixMilli(),
	}

	if isBusy {
		targetSession.Lock()
		targetSession.Queue = append(targetSession.Queue, task)
		qLen := len(targetSession.Queue)
		targetSession.Unlock()

		fmt.Printf("[WS] ⏳ Session %s busy. Adding to queue (%d waiting)\n", targetSession.ID, qLen)
		targetSession.Broadcast(map[string]interface{}{
			"type":        "message_queued",
			"messageId":   msgID,
			"prompt":      userPrompt,
			"imageUrl":    firstImageURL,
			"queueLength": qLen,
		})
		return
	}

	go targetSession.ExecuteTurn(task)
}
