package channel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"xbot/config"
)

// ── Session name utilities ──

const defaultSessionName = "default"

// SessionChatID builds a chatID from workDir and session name.
// Old format (backward compat): when sessionName is "default", returns just workDir.
func SessionChatID(workDir, sessionName string) string {
	if sessionName == "" || sessionName == defaultSessionName {
		return workDir
	}
	return workDir + ":" + sessionName
}

// ParseChatID extracts the workDir and sessionName from a chatID.
// Returns (workDir, sessionName). If there's no ":" separator, sessionName is "default".
func ParseChatID(chatID string) (workDir, sessionName string) {
	idx := strings.LastIndex(chatID, ":")
	if idx <= 0 || idx == len(chatID)-1 {
		return chatID, defaultSessionName
	}
	// WorkDir is everything before the last ":", session name after.
	workDir = chatID[:idx]
	sessionName = chatID[idx+1:]
	// Validate: workDir should look like an absolute path
	if !strings.HasPrefix(workDir, "/") && !strings.HasPrefix(workDir, ".") {
		return chatID, defaultSessionName
	}
	return workDir, sessionName
}

// ── Per-directory session storage ──

// dirSessions stores the list of sessions for a given directory.
// Persisted to ~/.xbot/sessions/<hash>.json
type dirSessions struct {
	Dir      string       `json:"dir"`
	Sessions []dirSession `json:"sessions"`
}

type dirSession struct {
	Name      string    `json:"name"`
	ChatID    string    `json:"chat_id"`
	CreatedAt time.Time `json:"created_at"`
}

// sessionsDir returns the directory where per-directory session files are stored.
func sessionsDir() string {
	return filepath.Join(config.XbotHome(), "sessions")
}

// loadDirSessions loads the session list for a given work directory.
func loadDirSessions(workDir string) (*dirSessions, error) {
	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	hash := sessionDirHash(workDir)
	path := filepath.Join(dir, hash+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No sessions file yet — return empty list with default session
			return &dirSessions{
				Dir: workDir,
				Sessions: []dirSession{{
					Name:      defaultSessionName,
					ChatID:    workDir,
					CreatedAt: time.Now(),
				}},
			}, nil
		}
		return nil, err
	}

	var ds dirSessions
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("parse sessions file: %w", err)
	}
	ds.Dir = workDir
	// Ensure default session always exists
	if !ds.hasSession(defaultSessionName) {
		ds.Sessions = append([]dirSession{{
			Name:      defaultSessionName,
			ChatID:    workDir,
			CreatedAt: time.Now(),
		}}, ds.Sessions...)
	}
	return &ds, nil
}

// saveDirSessions persists the session list to disk.
func (ds *dirSessions) save() error {
	dir := sessionsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	hash := sessionDirHash(ds.Dir)
	path := filepath.Join(dir, hash+".json")
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (ds *dirSessions) hasSession(name string) bool {
	for _, s := range ds.Sessions {
		if s.Name == name {
			return true
		}
	}
	return false
}

// addSession adds a new session to the directory.
func (ds *dirSessions) addSession(name string) (string, error) {
	if ds.hasSession(name) {
		return "", fmt.Errorf("session %q already exists", name)
	}
	chatID := SessionChatID(ds.Dir, name)
	ds.Sessions = append(ds.Sessions, dirSession{
		Name:      name,
		ChatID:    chatID,
		CreatedAt: time.Now(),
	})
	return chatID, ds.save()
}

// removeSession removes a session (except "default").
func (ds *dirSessions) removeSession(name string) error {
	if name == defaultSessionName {
		return fmt.Errorf("cannot delete default session")
	}
	for i, s := range ds.Sessions {
		if s.Name == name {
			ds.Sessions = append(ds.Sessions[:i], ds.Sessions[i+1:]...)
			return ds.save()
		}
	}
	return fmt.Errorf("session %q not found", name)
}

// sortedSessions returns sessions sorted by creation time.
func (ds *dirSessions) sortedSessions() []dirSession {
	sorted := make([]dirSession, len(ds.Sessions))
	copy(sorted, ds.Sessions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})
	return sorted
}

// sessionDirHash creates a safe filename hash from a directory path.
// Uses simple character substitution to avoid collision issues.
func sessionDirHash(workDir string) string {
	abs, _ := filepath.Abs(workDir)
	// Remove trailing separators
	abs = strings.TrimRight(abs, string(filepath.Separator))
	// Replace path separators and colons
	hash := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		" ", "_",
	).Replace(abs)
	// Remove leading underscore from absolute paths
	hash = strings.TrimPrefix(hash, "_")
	return hash
}

// listLocalDirSessions returns all sessions in the current directory from
// the local session store (used by the sessions panel).
func (m *cliModel) listLocalDirSessions() []SessionPanelEntry {
	ds, err := loadDirSessions(m.workDir)
	if err != nil {
		return nil
	}
	var entries []SessionPanelEntry
	for _, s := range ds.sortedSessions() {
		active := s.ChatID == m.chatID
		entries = append(entries, SessionPanelEntry{
			ID:      s.ChatID,
			Label:   s.Name,
			Type:    "main",
			Channel: "cli",
			Active:  active,
		})
	}
	return entries
}
