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

	fmt.Printf("[DB] SQLite database initialized successfully at: %s\n", dbPath)
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
