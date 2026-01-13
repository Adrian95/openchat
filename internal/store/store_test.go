package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestDB(t *testing.T) (*Store, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		store.Close()
		os.Remove(dbPath)
	}

	return store, cleanup
}

func TestNewStore(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	if store == nil {
		t.Fatal("store should not be nil")
	}
}

func TestCreateSession(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "You are a helpful assistant.")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if session.Name != "Test Session" {
		t.Errorf("expected name 'Test Session', got '%s'", session.Name)
	}
	if session.Provider != "openai" {
		t.Errorf("expected provider 'openai', got '%s'", session.Provider)
	}
	if session.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got '%s'", session.Model)
	}
	if session.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("unexpected system prompt: %s", session.SystemPrompt)
	}
}

func TestGetSession(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create a session
	created, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Retrieve it
	retrieved, err := store.GetSession(created.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("retrieved session should not be nil")
	}
	if retrieved.ID != created.ID {
		t.Errorf("expected ID '%s', got '%s'", created.ID, retrieved.ID)
	}
	if retrieved.Name != created.Name {
		t.Errorf("expected name '%s', got '%s'", created.Name, retrieved.Name)
	}

	// Test non-existent session
	nonExistent, err := store.GetSession("non-existent-id")
	if err != nil {
		t.Fatalf("GetSession should not error for non-existent: %v", err)
	}
	if nonExistent != nil {
		t.Error("non-existent session should return nil")
	}
}

func TestListSessions(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// Create multiple sessions
	_, err := store.CreateSession("Session 1", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond) // Ensure different timestamps

	_, err = store.CreateSession("Session 2", "anthropic", "claude-3-5-sonnet-20241022", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}

	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Should be ordered by updated_at DESC (Session 2 first)
	if sessions[0].Name != "Session 2" {
		t.Errorf("expected first session 'Session 2', got '%s'", sessions[0].Name)
	}
}

func TestUpdateSession(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Original Name", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	session.Name = "Updated Name"
	session.Model = "gpt-4-turbo"

	if err := store.UpdateSession(session); err != nil {
		t.Fatalf("UpdateSession failed: %v", err)
	}

	updated, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Model != "gpt-4-turbo" {
		t.Errorf("expected model 'gpt-4-turbo', got '%s'", updated.Model)
	}
}

func TestDeleteSession(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("To Delete", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add a message to test cascade delete
	_, err = store.AddMessage(session.ID, RoleUser, "Hello")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	if err := store.DeleteSession(session.ID); err != nil {
		t.Fatalf("DeleteSession failed: %v", err)
	}

	// Session should be gone
	deleted, err := store.GetSession(session.ID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if deleted != nil {
		t.Error("deleted session should return nil")
	}

	// Messages should be cascade deleted
	messages, err := store.GetMessages(session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", len(messages))
	}
}

func TestAddMessage(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	msg, err := store.AddMessage(session.ID, RoleUser, "Hello, world!")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	if msg.ID == "" {
		t.Error("message ID should not be empty")
	}
	if msg.SessionID != session.ID {
		t.Errorf("expected session ID '%s', got '%s'", session.ID, msg.SessionID)
	}
	if msg.Role != RoleUser {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}
	if msg.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got '%s'", msg.Content)
	}
}

func TestGetMessages(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add messages
	_, err = store.AddMessage(session.ID, RoleUser, "Hello")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	_, err = store.AddMessage(session.ID, RoleAssistant, "Hi there!")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	messages, err := store.GetMessages(session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}

	// Should be in chronological order
	if messages[0].Role != RoleUser {
		t.Errorf("expected first message role 'user', got '%s'", messages[0].Role)
	}
	if messages[1].Role != RoleAssistant {
		t.Errorf("expected second message role 'assistant', got '%s'", messages[1].Role)
	}
}

func TestGetLastNMessages(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Add 5 messages
	for i := 1; i <= 5; i++ {
		_, err := store.AddMessage(session.ID, RoleUser, "Message "+string(rune('0'+i)))
		if err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Get last 3
	messages, err := store.GetLastNMessages(session.ID, 3)
	if err != nil {
		t.Fatalf("GetLastNMessages failed: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}

	// Should be in chronological order (3, 4, 5)
	if messages[0].Content != "Message 3" {
		t.Errorf("expected 'Message 3', got '%s'", messages[0].Content)
	}
	if messages[2].Content != "Message 5" {
		t.Errorf("expected 'Message 5', got '%s'", messages[2].Content)
	}
}

func TestUpdateMessage(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	msg, err := store.AddMessage(session.ID, RoleAssistant, "Original content")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	if err := store.UpdateMessage(msg.ID, "Updated content"); err != nil {
		t.Fatalf("UpdateMessage failed: %v", err)
	}

	messages, err := store.GetMessages(session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if messages[0].Content != "Updated content" {
		t.Errorf("expected 'Updated content', got '%s'", messages[0].Content)
	}
}

func TestDeleteMessage(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	msg, err := store.AddMessage(session.ID, RoleUser, "To delete")
	if err != nil {
		t.Fatalf("AddMessage failed: %v", err)
	}

	if err := store.DeleteMessage(msg.ID); err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	messages, err := store.GetMessages(session.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
}

func TestGetMessageCount(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test Session", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	count, err := store.GetMessageCount(session.ID)
	if err != nil {
		t.Fatalf("GetMessageCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Add messages
	for i := 0; i < 5; i++ {
		_, err := store.AddMessage(session.ID, RoleUser, "Message")
		if err != nil {
			t.Fatalf("AddMessage failed: %v", err)
		}
	}

	count, err = store.GetMessageCount(session.ID)
	if err != nil {
		t.Fatalf("GetMessageCount failed: %v", err)
	}
	if count != 5 {
		t.Errorf("expected 5, got %d", count)
	}
}

func TestSearchSessions(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := store.CreateSession("Golang Tutorial", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = store.CreateSession("Python Basics", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	_, err = store.CreateSession("Go Advanced", "anthropic", "claude-3-5-sonnet-20241022", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Search for "Go"
	results, err := store.SearchSessions("Go")
	if err != nil {
		t.Fatalf("SearchSessions failed: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestGetMostRecentSession(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	// No sessions
	recent, err := store.GetMostRecentSession()
	if err != nil {
		t.Fatalf("GetMostRecentSession failed: %v", err)
	}
	if recent != nil {
		t.Error("expected nil for empty database")
	}

	// Create sessions
	_, err = store.CreateSession("First", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	second, err := store.CreateSession("Second", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	recent, err = store.GetMostRecentSession()
	if err != nil {
		t.Fatalf("GetMostRecentSession failed: %v", err)
	}

	if recent.ID != second.ID {
		t.Errorf("expected most recent to be 'Second', got '%s'", recent.Name)
	}
}

func TestMigrations(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "migration_test.db")

	// First open - should apply all migrations
	store1, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store1.Close()

	// Second open - should not re-apply migrations
	store2, err := New(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen store: %v", err)
	}
	defer store2.Close()

	// Should still work
	_, err = store2.CreateSession("Test", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed after reopen: %v", err)
	}
}

func TestRoleValidation(t *testing.T) {
	store, cleanup := setupTestDB(t)
	defer cleanup()

	session, err := store.CreateSession("Test", "openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Valid roles should work
	validRoles := []Role{RoleSystem, RoleUser, RoleAssistant, RoleTool}
	for _, role := range validRoles {
		_, err := store.AddMessage(session.ID, role, "Test message")
		if err != nil {
			t.Errorf("AddMessage failed for valid role '%s': %v", role, err)
		}
	}

	// Invalid role should fail (SQLite CHECK constraint)
	_, err = store.AddMessage(session.ID, Role("invalid"), "Test")
	if err == nil {
		t.Error("expected error for invalid role")
	}
}
