package store

import (
	"database/sql"
	"fmt"
	"strings"
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
	RoleSummary   Role = "summary"
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

// Attachment represents a file attached to a session context vault
type Attachment struct {
	ID                string
	SessionID         string
	Filename          string
	Filepath          string
	Content           string
	SizeBytes         int64
	MimeType          string
	IncludedInContext bool
	CreatedAt         time.Time
}

// Summary represents a summarized portion of conversation history
type Summary struct {
	ID                 string
	SessionID          string
	StartMessageID     string
	EndMessageID       string
	SummaryContent     string
	OriginalTokenCount int
	SummaryTokenCount  int
	CreatedAt          time.Time
}

// SearchResult represents a search result from FTS5
type SearchResult struct {
	SessionID    string
	SessionName  string
	MessageID    string
	Content      string
	Snippet      string
	Role         Role
	MatchRank    float64
	CreatedAt    time.Time
}

// Store provides database operations for sessions and messages
type Store struct {
	db          *sql.DB
	hasFTS5     bool // Whether FTS5 is available
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

	// Check if FTS5 is available
	s.hasFTS5 = s.checkFTS5Available()

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

		// Skip FTS5 migrations if FTS5 is not available (versions 7-14)
		// These are: messages_fts, sessions_fts, and their triggers
		if !s.hasFTS5 && version >= 7 && version <= 14 {
			// Record that we skipped this migration
			if _, err := s.db.Exec(insertSchemaVersionSQL, version); err != nil {
				return fmt.Errorf("failed to record skipped migration %d: %w", version, err)
			}
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

// checkFTS5Available checks if FTS5 is available in SQLite
func (s *Store) checkFTS5Available() bool {
	// Try to create a temporary FTS5 table
	_, err := s.db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS _fts5_test USING fts5(content)")
	if err != nil {
		return false
	}
	// Clean up the test table
	s.db.Exec("DROP TABLE IF EXISTS _fts5_test")
	return true
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

// GetMessage retrieves a message by ID
func (s *Store) GetMessage(id string) (*Message, error) {
	msg := &Message{}
	err := s.db.QueryRow(`
		SELECT id, session_id, role, content, created_at
		FROM messages WHERE id = ?
	`, id).Scan(&msg.ID, &msg.SessionID, &msg.Role, &msg.Content, &msg.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	return msg, nil
}

// FullTextSearch searches messages and sessions using FTS5 or fallback LIKE
func (s *Store) FullTextSearch(query string, limit int) ([]*SearchResult, error) {
	if limit <= 0 {
		limit = 50
	}

	// Use FTS5 if available, otherwise fall back to LIKE
	if s.hasFTS5 {
		return s.fullTextSearchFTS5(query, limit)
	}
	return s.fullTextSearchLike(query, limit)
}

// fullTextSearchFTS5 uses FTS5 for full-text search
func (s *Store) fullTextSearchFTS5(query string, limit int) ([]*SearchResult, error) {
	rows, err := s.db.Query(`
		SELECT
			m.id,
			m.session_id,
			s.name,
			m.role,
			m.content,
			snippet(messages_fts, 2, '>>>', '<<<', '...', 64) as snippet,
			rank
		FROM messages_fts
		JOIN messages m ON messages_fts.message_id = m.id
		JOIN sessions s ON m.session_id = s.id
		WHERE messages_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		r := &SearchResult{}
		var role string
		err := rows.Scan(&r.MessageID, &r.SessionID, &r.SessionName, &role, &r.Content, &r.Snippet, &r.MatchRank)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		r.Role = Role(role)
		results = append(results, r)
	}

	return results, rows.Err()
}

// fullTextSearchLike uses LIKE for search when FTS5 is not available
func (s *Store) fullTextSearchLike(query string, limit int) ([]*SearchResult, error) {
	likeQuery := "%" + query + "%"
	rows, err := s.db.Query(`
		SELECT
			m.id,
			m.session_id,
			s.name,
			m.role,
			m.content,
			m.created_at
		FROM messages m
		JOIN sessions s ON m.session_id = s.id
		WHERE m.content LIKE ?
		ORDER BY m.created_at DESC
		LIMIT ?
	`, likeQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search messages: %w", err)
	}
	defer rows.Close()

	var results []*SearchResult
	for rows.Next() {
		r := &SearchResult{}
		var role string
		err := rows.Scan(&r.MessageID, &r.SessionID, &r.SessionName, &role, &r.Content, &r.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan search result: %w", err)
		}
		r.Role = Role(role)
		// Create a simple snippet from content
		r.Snippet = createSnippet(r.Content, query, 100)
		results = append(results, r)
	}

	return results, rows.Err()
}

// createSnippet creates a snippet around the first occurrence of the query
func createSnippet(content, query string, maxLen int) string {
	lowerContent := strings.ToLower(content)
	lowerQuery := strings.ToLower(query)

	idx := strings.Index(lowerContent, lowerQuery)
	if idx < 0 {
		if len(content) > maxLen {
			return content[:maxLen] + "..."
		}
		return content
	}

	// Calculate start and end positions for snippet
	start := idx - 30
	if start < 0 {
		start = 0
	}
	end := idx + len(query) + 30
	if end > len(content) {
		end = len(content)
	}

	snippet := ""
	if start > 0 {
		snippet = "..."
	}
	snippet += content[start:end]
	if end < len(content) {
		snippet += "..."
	}

	return snippet
}

// SearchSessionsByFTS searches sessions using FTS5 or fallback LIKE
func (s *Store) SearchSessionsByFTS(query string, limit int) ([]*Session, error) {
	if limit <= 0 {
		limit = 20
	}

	// Use FTS5 if available, otherwise fall back to LIKE
	if s.hasFTS5 {
		rows, err := s.db.Query(`
			SELECT
				s.id, s.name, s.provider, s.model, s.system_prompt, s.created_at, s.updated_at
			FROM sessions_fts
			JOIN sessions s ON sessions_fts.session_id = s.id
			WHERE sessions_fts MATCH ?
			ORDER BY rank
			LIMIT ?
		`, query, limit)
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

	// Fallback to LIKE search
	likeQuery := "%" + query + "%"
	rows, err := s.db.Query(`
		SELECT id, name, provider, model, system_prompt, created_at, updated_at
		FROM sessions
		WHERE name LIKE ? OR system_prompt LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, likeQuery, likeQuery, limit)
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

// RebuildFTSIndex rebuilds the FTS index from existing data
func (s *Store) RebuildFTSIndex() error {
	if !s.hasFTS5 {
		// FTS5 not available, nothing to rebuild
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Rebuild messages FTS
	if _, err := tx.Exec("DELETE FROM messages_fts"); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to clear messages_fts: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO messages_fts(message_id, session_id, content)
		SELECT id, session_id, content FROM messages
	`); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to rebuild messages_fts: %w", err)
	}

	// Rebuild sessions FTS
	if _, err := tx.Exec("DELETE FROM sessions_fts"); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to clear sessions_fts: %w", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO sessions_fts(session_id, name, system_prompt)
		SELECT id, name, system_prompt FROM sessions
	`); err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to rebuild sessions_fts: %w", err)
	}

	return tx.Commit()
}

// HasFTS5 returns whether FTS5 is available
func (s *Store) HasFTS5() bool {
	return s.hasFTS5
}

// AddAttachment adds a file to the session's context vault
func (s *Store) AddAttachment(sessionID, filename, filepath, content, mimeType string, sizeBytes int64) (*Attachment, error) {
	att := &Attachment{
		ID:                uuid.New().String(),
		SessionID:         sessionID,
		Filename:          filename,
		Filepath:          filepath,
		Content:           content,
		SizeBytes:         sizeBytes,
		MimeType:          mimeType,
		IncludedInContext: true,
		CreatedAt:         time.Now(),
	}

	_, err := s.db.Exec(`
		INSERT INTO attachments (id, session_id, filename, filepath, content, size_bytes, mime_type, included_in_context, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, att.ID, att.SessionID, att.Filename, att.Filepath, att.Content, att.SizeBytes, att.MimeType, att.IncludedInContext, att.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add attachment: %w", err)
	}

	return att, nil
}

// GetAttachments retrieves all attachments for a session
func (s *Store) GetAttachments(sessionID string) ([]*Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, filename, filepath, content, size_bytes, mime_type, included_in_context, created_at
		FROM attachments WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get attachments: %w", err)
	}
	defer rows.Close()

	var attachments []*Attachment
	for rows.Next() {
		att := &Attachment{}
		err := rows.Scan(&att.ID, &att.SessionID, &att.Filename, &att.Filepath, &att.Content,
			&att.SizeBytes, &att.MimeType, &att.IncludedInContext, &att.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attachment: %w", err)
		}
		attachments = append(attachments, att)
	}

	return attachments, rows.Err()
}

// GetActiveAttachments retrieves attachments marked for inclusion in context
func (s *Store) GetActiveAttachments(sessionID string) ([]*Attachment, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, filename, filepath, content, size_bytes, mime_type, included_in_context, created_at
		FROM attachments WHERE session_id = ? AND included_in_context = 1
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active attachments: %w", err)
	}
	defer rows.Close()

	var attachments []*Attachment
	for rows.Next() {
		att := &Attachment{}
		err := rows.Scan(&att.ID, &att.SessionID, &att.Filename, &att.Filepath, &att.Content,
			&att.SizeBytes, &att.MimeType, &att.IncludedInContext, &att.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan attachment: %w", err)
		}
		attachments = append(attachments, att)
	}

	return attachments, rows.Err()
}

// ToggleAttachmentContext toggles whether an attachment is included in context
func (s *Store) ToggleAttachmentContext(id string) error {
	_, err := s.db.Exec("UPDATE attachments SET included_in_context = NOT included_in_context WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to toggle attachment: %w", err)
	}
	return nil
}

// DeleteAttachment removes an attachment
func (s *Store) DeleteAttachment(id string) error {
	_, err := s.db.Exec("DELETE FROM attachments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete attachment: %w", err)
	}
	return nil
}

// GetAttachmentsTotalSize returns the total size of all attachments for a session
func (s *Store) GetAttachmentsTotalSize(sessionID string) (int64, error) {
	var total int64
	err := s.db.QueryRow("SELECT COALESCE(SUM(size_bytes), 0) FROM attachments WHERE session_id = ?", sessionID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get total attachments size: %w", err)
	}
	return total, nil
}

// AddSummary stores a summary of conversation history
func (s *Store) AddSummary(sessionID, startMsgID, endMsgID, content string, origTokens, summaryTokens int) (*Summary, error) {
	sum := &Summary{
		ID:                 uuid.New().String(),
		SessionID:          sessionID,
		StartMessageID:     startMsgID,
		EndMessageID:       endMsgID,
		SummaryContent:     content,
		OriginalTokenCount: origTokens,
		SummaryTokenCount:  summaryTokens,
		CreatedAt:          time.Now(),
	}

	_, err := s.db.Exec(`
		INSERT INTO summaries (id, session_id, start_message_id, end_message_id, summary_content, original_token_count, summary_token_count, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sum.ID, sum.SessionID, sum.StartMessageID, sum.EndMessageID, sum.SummaryContent, sum.OriginalTokenCount, sum.SummaryTokenCount, sum.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to add summary: %w", err)
	}

	return sum, nil
}

// GetSummaries retrieves all summaries for a session
func (s *Store) GetSummaries(sessionID string) ([]*Summary, error) {
	rows, err := s.db.Query(`
		SELECT id, session_id, start_message_id, end_message_id, summary_content, original_token_count, summary_token_count, created_at
		FROM summaries WHERE session_id = ?
		ORDER BY created_at ASC
	`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get summaries: %w", err)
	}
	defer rows.Close()

	var summaries []*Summary
	for rows.Next() {
		sum := &Summary{}
		err := rows.Scan(&sum.ID, &sum.SessionID, &sum.StartMessageID, &sum.EndMessageID,
			&sum.SummaryContent, &sum.OriginalTokenCount, &sum.SummaryTokenCount, &sum.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}
		summaries = append(summaries, sum)
	}

	return summaries, rows.Err()
}

// GetLatestSummary retrieves the most recent summary for a session
func (s *Store) GetLatestSummary(sessionID string) (*Summary, error) {
	sum := &Summary{}
	err := s.db.QueryRow(`
		SELECT id, session_id, start_message_id, end_message_id, summary_content, original_token_count, summary_token_count, created_at
		FROM summaries WHERE session_id = ?
		ORDER BY created_at DESC LIMIT 1
	`, sessionID).Scan(&sum.ID, &sum.SessionID, &sum.StartMessageID, &sum.EndMessageID,
		&sum.SummaryContent, &sum.OriginalTokenCount, &sum.SummaryTokenCount, &sum.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest summary: %w", err)
	}

	return sum, nil
}

// DeleteSummary removes a summary
func (s *Store) DeleteSummary(id string) error {
	_, err := s.db.Exec("DELETE FROM summaries WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete summary: %w", err)
	}
	return nil
}

// GetMessagesAfterID retrieves all messages after a specific message ID (chronologically)
func (s *Store) GetMessagesAfterID(sessionID, messageID string) ([]*Message, error) {
	// First get the timestamp of the reference message
	var refTime time.Time
	err := s.db.QueryRow("SELECT created_at FROM messages WHERE id = ?", messageID).Scan(&refTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get reference message: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, created_at
		FROM messages
		WHERE session_id = ? AND created_at > ?
		ORDER BY created_at ASC
	`, sessionID, refTime)
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

// GetMessagesInRange retrieves messages between two message IDs (inclusive)
func (s *Store) GetMessagesInRange(sessionID, startMsgID, endMsgID string) ([]*Message, error) {
	var startTime, endTime time.Time
	err := s.db.QueryRow("SELECT created_at FROM messages WHERE id = ?", startMsgID).Scan(&startTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get start message: %w", err)
	}
	err = s.db.QueryRow("SELECT created_at FROM messages WHERE id = ?", endMsgID).Scan(&endTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get end message: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT id, session_id, role, content, created_at
		FROM messages
		WHERE session_id = ? AND created_at >= ? AND created_at <= ?
		ORDER BY created_at ASC
	`, sessionID, startTime, endTime)
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
