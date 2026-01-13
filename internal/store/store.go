package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Role represents the role of a message sender
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Session represents a chat session
type Session struct {
	ID           string
	Name         string
	Provider     string
	Model        string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Message represents a single message in a session
type Message struct {
	ID        string
	SessionID string
	Role      Role
	Content   string
	CreatedAt time.Time
}

// Store provides database operations for sessions and messages
type Store struct {
	db *sql.DB
}

// New creates a new Store instance and initializes the database
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent access
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	store := &Store{db: db}

	if err := store.runMigrations(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return store, nil
}

// runMigrations applies any pending database migrations
func (s *Store) runMigrations() error {
	// Create schema_version table if it doesn't exist
	// (this is a bootstrap - the table is also in migrations)
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	// Get current schema version
	var currentVersion int
	err = s.db.QueryRow(getSchemaVersionSQL).Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	// Apply pending migrations
	for i, migration := range migrations {
		version := i + 1
		if version <= currentVersion {
			continue
		}

		tx, err := s.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(migration); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", version, err)
		}

		if _, err := tx.Exec(insertSchemaVersionSQL, version); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %d: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", version, err)
		}
	}

	return nil
}

// Close closes the database connection
func (s *Store) Close() error {
	return s.db.Close()
}

// CreateSession creates a new chat session
func (s *Store) CreateSession(name, provider, model, systemPrompt string) (*Session, error) {
	session := &Session{
		ID:           uuid.New().String(),
		Name:         name,
		Provider:     provider,
		Model:        model,
		SystemPrompt: systemPrompt,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := s.db.Exec(`
		INSERT INTO sessions (id, name, provider, model, system_prompt, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, session.ID, session.Name, session.Provider, session.Model, session.SystemPrompt,
		session.CreatedAt, session.UpdatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetSession retrieves a session by ID
func (s *Store) GetSession(id string) (*Session, error) {
	session := &Session{}
	err := s.db.QueryRow(`
		SELECT id, name, provider, model, system_prompt, created_at, updated_at
		FROM sessions WHERE id = ?
	`, id).Scan(&session.ID, &session.Name, &session.Provider, &session.Model,
		&session.SystemPrompt, &session.CreatedAt, &session.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// ListSessions returns all sessions ordered by most recently updated
func (s *Store) ListSessions() ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, name, provider, model, system_prompt, created_at, updated_at
		FROM sessions ORDER BY updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		err := rows.Scan(&session.ID, &session.Name, &session.Provider, &session.Model,
			&session.SystemPrompt, &session.CreatedAt, &session.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// UpdateSession updates an existing session
func (s *Store) UpdateSession(session *Session) error {
	session.UpdatedAt = time.Now()
	_, err := s.db.Exec(`
		UPDATE sessions SET name = ?, provider = ?, model = ?, system_prompt = ?, updated_at = ?
		WHERE id = ?
	`, session.Name, session.Provider, session.Model, session.SystemPrompt,
		session.UpdatedAt, session.ID)

	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession deletes a session and its messages
func (s *Store) DeleteSession(id string) error {
	_, err := s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}
	return nil
}

// AddMessage adds a message to a session
func (s *Store) AddMessage(sessionID string, role Role, content string) (*Message, error) {
	msg := &Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO messages (id, session_id, role, content, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, msg.ID, msg.SessionID, msg.Role, msg.Content, msg.CreatedAt)

	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to add message: %w", err)
	}

	// Update session's updated_at
	_, err = tx.Exec("UPDATE sessions SET updated_at = ? WHERE id = ?",
		time.Now(), sessionID)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to update session timestamp: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return msg, nil
}

// GetMessages retrieves all messages for a session in chronological order
func (s *Store) GetMessages(sessionID string) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, created_at
		FROM messages WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// GetLastNMessages retrieves the last N messages for a session
func (s *Store) GetLastNMessages(sessionID string, n int) ([]*Message, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, created_at
		FROM messages WHERE session_id = ?
		ORDER BY created_at DESC LIMIT ?
	`, sessionID, n)
	if err != nil {
		return nil, fmt.Errorf("failed to get messages: %w", err)
	}
	defer rows.Close()

	var messages []*Message
	for rows.Next() {
		msg := &Message{}
		err := rows.Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// UpdateMessage updates an existing message's content
func (s *Store) UpdateMessage(id, content string) error {
	_, err := s.db.Exec("UPDATE messages SET content = ? WHERE id = ?", content, id)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}
	return nil
}

// DeleteMessage deletes a message
func (s *Store) DeleteMessage(id string) error {
	_, err := s.db.Exec("DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}
	return nil
}

// GetMessageCount returns the number of messages in a session
func (s *Store) GetMessageCount(sessionID string) (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id = ?", sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count messages: %w", err)
	}
	return count, nil
}

// SearchSessions searches sessions by name (case-insensitive)
func (s *Store) SearchSessions(query string) ([]*Session, error) {
	rows, err := s.db.Query(`
		SELECT id, name, provider, model, system_prompt, created_at, updated_at
		FROM sessions WHERE name LIKE ? ORDER BY updated_at DESC
	`, "%"+query+"%")
	if err != nil {
		return nil, fmt.Errorf("failed to search sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		err := rows.Scan(&session.ID, &session.Name, &session.Provider, &session.Model,
			&session.SystemPrompt, &session.CreatedAt, &session.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}
		sessions = append(sessions, session)
	}

	return sessions, rows.Err()
}

// GetMostRecentSession returns the most recently updated session
func (s *Store) GetMostRecentSession() (*Session, error) {
	session := &Session{}
	err := s.db.QueryRow(`
		SELECT id, name, provider, model, system_prompt, created_at, updated_at
		FROM sessions ORDER BY updated_at DESC LIMIT 1
	`).Scan(&session.ID, &session.Name, &session.Provider, &session.Model,
		&session.SystemPrompt, &session.CreatedAt, &session.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get most recent session: %w", err)
	}

	return session, nil
}
