package runnerclient

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathGuard encapsulates path validation logic.
type PathGuard struct {
	// Workspace is the root workspace path
	Workspace string
	// FullControl disables all path restrictions
	FullControl bool
	// DockerMode: in Docker mode, only does string-level prefix checks
	DockerMode bool
}

// Validate checks if path is within the workspace; returns error if not.
// Skips all checks when FullControl is true.
// In DockerMode, only does string-level prefix check (no EvalSymlinks).
func (pg *PathGuard) Validate(path string) error {
	if pg.FullControl || pg.DockerMode {
		return nil
	}

	ws := pg.Workspace
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(ws, cleaned)
	}

	// Native mode: keep EvalSymlinks check
	real, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// File may not exist yet (e.g. write target), use cleaned path as fallback
		real = cleaned
	}

	if !strings.HasPrefix(real, ws) {
		return fmt.Errorf("path %q (resolved to %q) escapes workspace %q", path, real, ws)
	}
	return nil
}

// SafePath returns a cleaned and validated absolute path.
// When FullControl is true, only returns the cleaned path.
func (pg *PathGuard) SafePath(path string) (string, error) {
	ws := pg.Workspace
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(ws, cleaned)
	}
	if err := pg.Validate(path); err != nil {
		return "", err
	}
	return cleaned, nil
}
