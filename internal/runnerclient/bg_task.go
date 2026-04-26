package runnerclient

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xbot/internal/runnerproto"
)

// bgTask represents a background task.
type bgTask struct {
	id        string
	command   string
	req       runnerproto.BgExecRequest
	cmd       *exec.Cmd
	mu        sync.Mutex
	stdout    bytes.Buffer
	stderr    bytes.Buffer
	exitCode  int
	status    string // "running", "completed", "failed", "killed"
	startedAt time.Time
}

// bgTaskManager manages background tasks.
type bgTaskManager struct {
	mu      sync.RWMutex
	tasks   map[string]*bgTask
	verbose bool

	// used to build docker commands
	dockerMode bool
	workspace  string
	executor   Executor
	logf       LogFunc
}

func newBgTaskManager(verbose, dockerMode bool, workspace string, logf LogFunc) *bgTaskManager {
	return &bgTaskManager{
		tasks:      make(map[string]*bgTask),
		verbose:    verbose,
		dockerMode: dockerMode,
		workspace:  workspace,
		logf:       logf,
	}
}

// Start starts a background command (native mode runs in background, docker mode wraps in goroutine).
func (m *bgTaskManager) Start(req runnerproto.BgExecRequest) (*runnerproto.BgStartedResponse, error) {
	t := &bgTask{
		id:        req.TaskID,
		command:   req.Command,
		req:       req,
		status:    "running",
		startedAt: time.Now(),
	}

	m.mu.Lock()
	m.tasks[req.TaskID] = t
	m.mu.Unlock()

	go t.run(m)

	callLogf(m.logf, "  bg_exec started [id=%s]: %s", req.TaskID, req.Command)
	return &runnerproto.BgStartedResponse{TaskID: req.TaskID}, nil
}

// run executes the command and updates task status on completion.
func (t *bgTask) run(m *bgTaskManager) {
	var exitCode int
	var status string

	if m.dockerMode {
		exitCode, status = t.runDocker(m)
	} else {
		exitCode, status = t.runNative(m)
	}

	t.mu.Lock()
	t.exitCode = exitCode
	t.status = status
	t.mu.Unlock()

	callLogf(m.logf, "  bg_exec done [id=%s] status=%s exit=%d stdout=%dB stderr=%dB",
		t.id, t.status, t.exitCode, t.stdout.Len(), t.stderr.Len())
}

// runNative executes a command natively with process group support.
func (t *bgTask) runNative(m *bgTaskManager) (int, string) {
	var cmd *exec.Cmd
	if t.req.Shell {
		cmd = exec.Command("sh", "-c", t.req.Command)
	} else {
		if len(t.req.Args) == 0 {
			return -1, "failed"
		}
		cmd = exec.Command(t.req.Args[0], t.req.Args[1:]...)
	}

	// Create process group to kill entire process tree
	setProcessAttrs(cmd)

	dir := t.req.Dir
	if dir == "" {
		dir = m.workspace
	}
	cmd.Dir = filepath.Clean(dir)

	if len(t.req.Env) > 0 {
		cmd.Env = append(getBaseEnv(), t.req.Env...)
	}
	if t.req.Stdin != "" {
		cmd.Stdin = strings.NewReader(t.req.Stdin)
	}

	cmd.Stdout = &t.stdout
	cmd.Stderr = &t.stderr
	t.cmd = cmd

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), "failed"
		}
		return -1, "failed"
	}
	return 0, "completed"
}

// runDocker executes a command synchronously inside a Docker container.
func (t *bgTask) runDocker(m *bgTaskManager) (int, string) {
	de := m.executor.(*DockerExecutor)

	if t.req.Shell {
		args := []string{"exec", "-i", de.ContainerName, "sh", "-c", t.req.Command}
		return t.dockerRun(de, args, t.req.Stdin)
	}

	if len(t.req.Args) == 0 {
		return -1, "failed"
	}
	args := append([]string{"exec", "-i", de.ContainerName}, t.req.Args...)
	return t.dockerRun(de, args, t.req.Stdin)
}

// dockerRun executes a docker command and captures its output.
func (t *bgTask) dockerRun(de *DockerExecutor, args []string, stdin string) (int, string) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = de.HostWorkspace
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	cmd.Stdout = &t.stdout
	cmd.Stderr = &t.stderr
	t.cmd = cmd

	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode(), "failed"
		}
		return -1, "failed"
	}
	return 0, "completed"
}

// Kill sends SIGKILL to the background task's process group (native) or docker exec process (docker).
func (m *bgTaskManager) Kill(req runnerproto.BgKillRequest) error {
	m.mu.RLock()
	t, ok := m.tasks[req.TaskID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("task %s not found", req.TaskID)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.status != "running" {
		return fmt.Errorf("task %s is not running (status=%s)", req.TaskID, t.status)
	}

	if t.cmd != nil && t.cmd.Process != nil {
		if m.dockerMode {
			t.cmd.Process.Kill()
		} else {
			// Kill entire process group
			killProcessTree(t.cmd.Process.Pid)
		}
		t.status = "killed"
		callLogf(m.logf, "  bg_kill [id=%s]: killed", req.TaskID)
	}

	return nil
}

// Status returns the background task's current status and output snapshot.
func (m *bgTaskManager) Status(req runnerproto.BgStatusRequest) (*runnerproto.BgOutputResponse, error) {
	m.mu.RLock()
	t, ok := m.tasks[req.TaskID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("task %s not found", req.TaskID)
	}

	t.mu.Lock()
	resp := &runnerproto.BgOutputResponse{
		TaskID:   t.id,
		Status:   t.status,
		ExitCode: t.exitCode,
		Stdout:   t.stdout.String(),
		Stderr:   t.stderr.String(),
	}
	t.mu.Unlock()

	return resp, nil
}

// Cleanup kills all running background tasks (called on disconnect).
func (m *bgTaskManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, t := range m.tasks {
		t.mu.Lock()
		if t.status == "running" && t.cmd != nil && t.cmd.Process != nil {
			if m.dockerMode {
				t.cmd.Process.Kill()
			} else {
				killProcessTree(t.cmd.Process.Pid)
			}
			t.status = "killed"
		}
		t.mu.Unlock()
		delete(m.tasks, id)
	}
	callLogf(m.logf, "  bg_tasks: cleaned up all tasks on disconnect")
}

// getBaseEnv returns the base environment for native command execution.
func getBaseEnv() []string {
	return nil // exec.Command uses os.Environ by default when Env is nil
}
