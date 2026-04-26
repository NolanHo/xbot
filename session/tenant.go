package session

import (
	"context"
	"fmt"
	"sync"
	"time"

	"xbot/llm"
	"xbot/memory"
	"xbot/storage/sqlite"
	"xbot/tools"
)

// TenantSession represents a single tenant's conversation session
type TenantSession struct {
	tenantID   int64
	channel    string
	chatID     string
	sessionSvc *sqlite.SessionService
	memorySvc  *sqlite.MemoryService // for consolidation state (LastConsolidated)
	memory     memory.MemoryProvider
	mcpManager *tools.SessionMCPManager // session MCP manager
	lastActive time.Time                // session last active time
	mu         sync.RWMutex             // guards lastActive and cwd
	cwd        string                   // current working directory (PWD tool optimization)
}

// AddMessage adds a message to this tenant's session
func (s *TenantSession) AddMessage(msg llm.ChatMessage) error {
	return s.sessionSvc.AddMessage(s.tenantID, msg)
}

// ReplaceToolMessage updates the most recent matching tool-role message.
// Empty toolName/toolCallID act as wildcards (match any).
func (s *TenantSession) ReplaceToolMessage(toolName, toolCallID, content string) error {
	return s.sessionSvc.ReplaceToolMessage(s.tenantID, toolName, toolCallID, content)
}

// GetHistory retrieves recent messages for LLM context window
func (s *TenantSession) GetHistory(maxMessages int) ([]llm.ChatMessage, error) {
	return s.sessionSvc.GetHistory(s.tenantID, maxMessages)
}

// GetMessages retrieves all messages for this tenant
func (s *TenantSession) GetMessages() ([]llm.ChatMessage, error) {
	return s.sessionSvc.GetAllMessages(s.tenantID)
}

// Len returns the number of messages in this tenant's session
func (s *TenantSession) Len() (int, error) {
	return s.sessionSvc.GetMessagesCount(s.tenantID)
}

// UserMessageCount returns the number of user-role messages (conversation turns).
func (s *TenantSession) UserMessageCount() (int, error) {
	return s.sessionSvc.GetUserMessageCount(s.tenantID)
}

// LastConsolidated returns the last consolidated message index
func (s *TenantSession) LastConsolidated() int {
	lastConsolidated, err := s.memorySvc.GetState(context.Background(), s.tenantID)
	if err != nil {
		// If error, return 0 as safe default
		return 0
	}
	return lastConsolidated
}

// SetLastConsolidated updates the last consolidated message index
func (s *TenantSession) SetLastConsolidated(n int) error {
	return s.memorySvc.SetState(context.Background(), s.tenantID, n)
}

// Clear removes all messages from this tenant's session
func (s *TenantSession) Clear() error {
	return s.sessionSvc.Clear(s.tenantID)
}

// PurgeOldMessages deletes messages older than the most recent `keepCount` messages.
// Returns the number of messages deleted.
func (s *TenantSession) PurgeOldMessages(keepCount int) (int64, error) {
	return s.sessionSvc.PurgeOldMessages(s.tenantID, keepCount)
}

// UpdateMessageContent updates the content of the Nth message (0-indexed) in this tenant's session.
// Used by observation masking to persist masked content back to session.
func (s *TenantSession) UpdateMessageContent(messageIndex int, content string) error {
	return s.sessionSvc.UpdateMessageContent(s.tenantID, messageIndex, content)
}

// Memory returns the memory provider for this tenant
func (s *TenantSession) Memory() memory.MemoryProvider {
	return s.memory
}

// TenantID returns the tenant ID
func (s *TenantSession) TenantID() int64 {
	return s.tenantID
}

// MemoryService returns the underlying SQLite memory service for this tenant.
// Used for tenant-level state operations (token state, consolidation state, etc.)
// that are independent of the memory provider implementation.
func (s *TenantSession) MemoryService() *sqlite.MemoryService {
	return s.memorySvc
}

// Channel returns the channel name
func (s *TenantSession) Channel() string {
	return s.channel
}

// ChatID returns the chat ID
func (s *TenantSession) ChatID() string {
	return s.chatID
}

// String returns a string representation of the tenant
func (s *TenantSession) String() string {
	return fmt.Sprintf("%s:%s (tenant_id=%d)", s.channel, s.chatID, s.tenantID)
}

// GetSessionKey returns the session's unique key
func (s *TenantSession) GetSessionKey() string {
	return s.channel + ":" + s.chatID
}

// MarkActive updates the session's last active time
func (s *TenantSession) MarkActive() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastActive = time.Now()
}

// SetMCPManager sets the session MCP manager
func (s *TenantSession) SetMCPManager(mgr *tools.SessionMCPManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcpManager = mgr
}

// GetMCPManager returns the MCP manager
func (s *TenantSession) GetMCPManager() *tools.SessionMCPManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mcpManager
}

// LastActive returns the session's last active time
func (s *TenantSession) LastActive() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastActive
}

// CleanupInactiveMCPs cleans up inactive MCP connections
// Returns the session's last active time (used to determine if session should be evicted from cache)
func (s *TenantSession) CleanupInactiveMCPs() time.Time {
	s.mu.RLock()
	mgr := s.mcpManager
	s.mu.RUnlock()

	if mgr != nil {
		return mgr.UnloadInactiveServers()
	}
	return s.LastActive()
}

// Close releases session resources
func (s *TenantSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mcpManager != nil {
		s.mcpManager.Close()
		s.mcpManager = nil
	}
}

// InvalidateMCP invalidates the session's MCP connections, forcing reload on next use
func (s *TenantSession) InvalidateMCP() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.mcpManager != nil {
		s.mcpManager.Invalidate()
	}
}

// GetCurrentDir returns the current working directory (PWD tool optimization)
func (s *TenantSession) GetCurrentDir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

// SetCurrentDir sets the current working directory (PWD tool optimization)
func (s *TenantSession) SetCurrentDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cwd = dir
}
