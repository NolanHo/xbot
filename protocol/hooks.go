package protocol

import (
	"context"
	"sync"
	"time"

	"xbot/tools"
)

type FileSnapshot struct {
	TurnIdx    int    `json:"turn_idx"`
	ToolName   string `json:"tool_name"`
	FilePath   string `json:"file_path"`
	Existed    bool   `json:"existed"`
	ContentB64 string `json:"content_b64,omitempty"`
}

type ApprovalRequest struct {
	ToolName    string `json:"tool_name"`
	ToolArgs    string `json:"tool_args"`
	RunAs       string `json:"run_as"`
	Reason      string `json:"reason,omitempty"`
	Command     string `json:"command,omitempty"`
	FilePath    string `json:"file_path,omitempty"`
	ArgsSummary string `json:"args_summary,omitempty"`
}

type ApprovalResult struct {
	Approved   bool   `json:"approved"`
	DenyReason string `json:"deny_reason,omitempty"`
}

type ApprovalHandler interface {
	RequestApproval(ctx context.Context, req ApprovalRequest) (ApprovalResult, error)
}

type ApprovalState struct {
	mu      sync.RWMutex
	handler tools.ApprovalHandler
	Timeout time.Duration `json:"timeout"`
}

// SetHandler replaces the approval handler at runtime.
func (s *ApprovalState) SetHandler(h tools.ApprovalHandler) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

// GetHandler returns the current approval handler.
func (s *ApprovalState) GetHandler() tools.ApprovalHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handler
}

type RewindResult struct {
	Restored   []string `json:"restored"`
	CreatedDel []string `json:"created_del"`
	Skipped    []string `json:"skipped"`
	Errors     []string `json:"errors"`
}

type CheckpointStore interface {
	Rewind(turnIdx int) (RewindResult, error)
	HasChanges(turnIdx int) bool
	CountChanges(turnIdx int) int
}

type CheckpointState struct {
	mu      sync.Mutex
	store   *tools.CheckpointStore
	turnIdx int
	pending map[string]tools.FileSnapshot
}

// NewCheckpointState creates a CheckpointState backed by the given store.
func NewCheckpointState(store *tools.CheckpointStore) *CheckpointState {
	return &CheckpointState{
		store:   store,
		pending: make(map[string]tools.FileSnapshot),
	}
}

// SetTurnIdx sets the current turn index. Should be called before each agent turn.
func (cs *CheckpointState) SetTurnIdx(idx int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.turnIdx = idx
}

// TurnIdx returns the current turn index.
func (cs *CheckpointState) TurnIdx() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.turnIdx
}

// Store returns the underlying CheckpointStore.
func (cs *CheckpointState) Store() *tools.CheckpointStore {
	return cs.store
}

// SetPending stores a file snapshot for the given path.
func (cs *CheckpointState) SetPending(filePath string, snap tools.FileSnapshot) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.pending[filePath] = snap
}

// GetAndDeletePending retrieves and removes the snapshot for the given path.
func (cs *CheckpointState) GetAndDeletePending(filePath string) (tools.FileSnapshot, bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	snap, found := cs.pending[filePath]
	if found {
		delete(cs.pending, filePath)
	}
	return snap, found
}
