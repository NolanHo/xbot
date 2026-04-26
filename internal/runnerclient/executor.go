package runnerclient

import (
	"context"
	"os"
	"time"
)

// Executor abstracts the runner's operation backend (native or docker).
type Executor interface {
	// Command execution
	Exec(ctx context.Context, spec ExecSpec) (*ExecResult, error)

	// File operations
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (FileInfo, error)
	ReadDir(path string) ([]DirEntry, error)
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error

	// Edge download
	DownloadFile(ctx context.Context, url, outputPath string) (int64, error)

	// Lifecycle
	Close() error
}

// ExecSpec holds command execution parameters.
type ExecSpec struct {
	Command string
	Args    []string
	Shell   bool
	Dir     string
	Env     []string
	Stdin   string
	Timeout time.Duration

	// RunAsUser is the OS username to execute the command as.
	// When set, the command is wrapped with: sudo -n -H -u <user> --
	// Requires NOPASSWD sudoers entry for the target user.
	RunAsUser string
}

// ExecResult holds the command execution result.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
}

// FileInfo holds file metadata (mirrors server-side SandboxFileInfo).
type FileInfo struct {
	Name    string
	Size    int64
	Mode    os.FileMode
	ModTime time.Time
	IsDir   bool
}

// DirEntry is a directory entry.
type DirEntry struct {
	Name  string
	IsDir bool
	Size  int64
}
