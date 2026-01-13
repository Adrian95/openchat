// Package store provides SQLite-based persistence for chat sessions and messages.
package store

// migrations contains SQL statements for database schema migrations.
// Each migration is run in order during database initialization.
var migrations = []string{
	// Migration 1: Create sessions table
	`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		provider TEXT NOT NULL,
		model TEXT NOT NULL,
		system_prompt TEXT DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,

	// Migration 2: Create messages table
	`CREATE TABLE IF NOT EXISTS messages (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		role TEXT NOT NULL CHECK(role IN ('system', 'user', 'assistant', 'tool', 'summary')),
		content TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,

	// Migration 3: Create indexes for efficient querying
	`CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id)`,
	`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at ON sessions(updated_at)`,

	// Migration 4: Create schema version table for tracking migrations
	`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`,

	// Migration 5: Create FTS5 virtual table for full-text search on messages
	`CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
		message_id,
		session_id,
		content,
		content=messages,
		content_rowid=rowid
	)`,

	// Migration 6: Create FTS5 virtual table for full-text search on sessions
	`CREATE VIRTUAL TABLE IF NOT EXISTS sessions_fts USING fts5(
		session_id,
		name,
		system_prompt,
		content=sessions,
		content_rowid=rowid
	)`,

	// Migration 7: Create triggers to keep FTS tables in sync with messages
	`CREATE TRIGGER IF NOT EXISTS messages_fts_insert AFTER INSERT ON messages BEGIN
		INSERT INTO messages_fts(message_id, session_id, content) VALUES (NEW.id, NEW.session_id, NEW.content);
	END`,

	// Migration 8: Create trigger for message updates
	`CREATE TRIGGER IF NOT EXISTS messages_fts_update AFTER UPDATE ON messages BEGIN
		DELETE FROM messages_fts WHERE message_id = OLD.id;
		INSERT INTO messages_fts(message_id, session_id, content) VALUES (NEW.id, NEW.session_id, NEW.content);
	END`,

	// Migration 9: Create trigger for message deletes
	`CREATE TRIGGER IF NOT EXISTS messages_fts_delete AFTER DELETE ON messages BEGIN
		DELETE FROM messages_fts WHERE message_id = OLD.id;
	END`,

	// Migration 10: Create triggers for sessions FTS
	`CREATE TRIGGER IF NOT EXISTS sessions_fts_insert AFTER INSERT ON sessions BEGIN
		INSERT INTO sessions_fts(session_id, name, system_prompt) VALUES (NEW.id, NEW.name, NEW.system_prompt);
	END`,

	// Migration 11: Create trigger for session updates
	`CREATE TRIGGER IF NOT EXISTS sessions_fts_update AFTER UPDATE ON sessions BEGIN
		DELETE FROM sessions_fts WHERE session_id = OLD.id;
		INSERT INTO sessions_fts(session_id, name, system_prompt) VALUES (NEW.id, NEW.name, NEW.system_prompt);
	END`,

	// Migration 12: Create trigger for session deletes
	`CREATE TRIGGER IF NOT EXISTS sessions_fts_delete AFTER DELETE ON sessions BEGIN
		DELETE FROM sessions_fts WHERE session_id = OLD.id;
	END`,

	// Migration 13: Create attachments table for per-session file context vault
	`CREATE TABLE IF NOT EXISTS attachments (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		filepath TEXT NOT NULL,
		content TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		mime_type TEXT DEFAULT '',
		included_in_context INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,

	// Migration 14: Create indexes for attachments
	`CREATE INDEX IF NOT EXISTS idx_attachments_session_id ON attachments(session_id)`,

	// Migration 15: Create summaries table for storing message summaries
	`CREATE TABLE IF NOT EXISTS summaries (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		start_message_id TEXT NOT NULL,
		end_message_id TEXT NOT NULL,
		summary_content TEXT NOT NULL,
		original_token_count INTEGER DEFAULT 0,
		summary_token_count INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	)`,

	// Migration 16: Create index for summaries
	`CREATE INDEX IF NOT EXISTS idx_summaries_session_id ON summaries(session_id)`,
}

// getSchemaVersion returns the current schema version
const getSchemaVersionSQL = `SELECT COALESCE(MAX(version), 0) FROM schema_version`

// insertSchemaVersion records a migration as applied
const insertSchemaVersionSQL = `INSERT INTO schema_version (version) VALUES (?)`
