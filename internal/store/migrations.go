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
		role TEXT NOT NULL CHECK(role IN ('system', 'user', 'assistant', 'tool')),
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
}

// getSchemaVersion returns the current schema version
const getSchemaVersionSQL = `SELECT COALESCE(MAX(version), 0) FROM schema_version`

// insertSchemaVersion records a migration as applied
const insertSchemaVersionSQL = `INSERT INTO schema_version (version) VALUES (?)`
