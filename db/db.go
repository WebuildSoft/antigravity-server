package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"antigravity-bridge/config"
	_ "modernc.org/sqlite"
)

var DB *sql.DB

func InitDB() error {
	dbDir := filepath.Join(config.AppConfig.UploadsDir, "..", "data")
	_ = os.MkdirAll(dbDir, 0755)
	dbPath := filepath.Join(dbDir, "antigravity.db")

	dsn := fmt.Sprintf("%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)", dbPath)
	var err error
	DB, err = sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("failed to open sqlite database: %v", err)
	}

	DB.SetMaxOpenConns(25)
	DB.SetMaxIdleConns(5)
	DB.SetConnMaxLifetime(10 * time.Minute)

	if err := createSchema(); err != nil {
		return err
	}

	if err := CleanupDuplicateMessages(); err != nil {
		fmt.Printf("[DB] Warning during duplicate cleanup: %v\n", err)
	}

	fmt.Printf("[DB] SQLite database initialized successfully at: %s\n", dbPath)
	return nil
}

func CleanupDuplicateMessages() error {
	// Clean duplicate user messages (where a client-generated ID already exists)
	userRes, err := DB.Exec(`
		DELETE FROM messages 
		WHERE role = 'user' 
		  AND id LIKE 'msg_%' 
		  AND EXISTS (
		    SELECT 1 FROM messages m2 
		    WHERE m2.conversation_id = messages.conversation_id 
		      AND m2.role = 'user' 
		      AND m2.content = messages.content 
		      AND m2.id != messages.id 
		      AND m2.id NOT LIKE 'msg_%'
		  );
	`)
	if err != nil {
		return err
	}

	// Clean duplicate agent messages (where a server-generated agent_ ID already exists)
	agentRes, err := DB.Exec(`
		DELETE FROM messages 
		WHERE role = 'agent' 
		  AND id LIKE 'msg_%' 
		  AND EXISTS (
		    SELECT 1 FROM messages m2 
		    WHERE m2.conversation_id = messages.conversation_id 
		      AND m2.role = 'agent' 
		      AND m2.id != messages.id 
		      AND m2.id LIKE 'agent_%'
		      AND (
		        m2.content = messages.content 
		        OR messages.content = '' 
		        OR (length(messages.content) > 30 AND substr(m2.content, 1, 30) = substr(messages.content, 1, 30))
		      )
		  );
	`)
	if err != nil {
		return err
	}

	userDeleted, _ := userRes.RowsAffected()
	agentDeleted, _ := agentRes.RowsAffected()
	if userDeleted > 0 || agentDeleted > 0 {
		fmt.Printf("[DB] Cleaned up duplicate messages: %d user, %d agent\n", userDeleted, agentDeleted)
	}
	return nil
}

func createSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS conversations (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		title TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		conversation_id TEXT NOT NULL,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		thinking TEXT,
		image_url TEXT,
		image_urls TEXT,
		created_at DATETIME NOT NULL,
		FOREIGN KEY(conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS tool_calls (
		id TEXT PRIMARY KEY,
		message_id TEXT NOT NULL,
		name TEXT NOT NULL,
		summary TEXT NOT NULL,
		args TEXT,
		output TEXT,
		status TEXT NOT NULL,
		FOREIGN KEY(message_id) REFERENCES messages(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_messages_conv ON messages(conversation_id, created_at ASC);
	CREATE INDEX IF NOT EXISTS idx_tool_calls_msg ON tool_calls(message_id);
	CREATE INDEX IF NOT EXISTS idx_conv_user ON conversations(user_id, updated_at DESC);
	`
	_, err := DB.Exec(schema)
	return err
}
