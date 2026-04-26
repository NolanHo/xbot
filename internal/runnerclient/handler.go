package runnerclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"xbot/internal/runnerproto"
	"xbot/llm"
)

// Handler processes requests received from the server.
type Handler struct {
	Executor        Executor
	PathGuard       *PathGuard
	LLMClient       llm.LLM
	LLMModels       []string
	LLMProviderName string // provider name for self-reporting (e.g. "openai", "anthropic")
	Verbose         bool

	// Internal management
	stdioMgr *stdioManager
	bgMgr    *bgTaskManager

	// Log callback (silent when nil)
	LogFunc LogFunc

	// Mode flags
	dockerMode bool
}

// HandlerOption is a functional option for Handler.
type HandlerOption func(*Handler)

// WithVerbose enables verbose logging.
func WithVerbose(v bool) HandlerOption {
	return func(h *Handler) { h.Verbose = v }
}

// WithPathGuard sets the PathGuard.
func WithPathGuard(pg *PathGuard) HandlerOption {
	return func(h *Handler) { h.PathGuard = pg }
}

// WithDockerMode enables Docker mode.
func WithDockerMode(v bool) HandlerOption {
	return func(h *Handler) { h.dockerMode = v }
}

// WithLogFunc sets the log callback (silent when nil).
func WithLogFunc(f LogFunc) HandlerOption {
	return func(h *Handler) { h.LogFunc = f }
}

// NewHandler creates a new Handler.
func NewHandler(exec Executor, opts ...HandlerOption) *Handler {
	h := &Handler{
		Executor: exec,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// InitLLM initializes the LLM client.
func (h *Handler) InitLLM(provider, baseURL, apiKey, model string) error {
	client, models, err := InitLLMClient(provider, baseURL, apiKey, model, h.LogFunc)
	if err != nil {
		return err
	}
	h.LLMClient = client
	h.LLMModels = models
	if client != nil {
		h.LLMProviderName = provider
	}
	return nil
}

// SetLLMClient directly sets the LLM client (for TUI runner to reuse an existing client).
// provider is used for the runner to self-report LLM capability (empty string = no LLM).
func (h *Handler) SetLLMClient(client llm.LLM, models []string, provider string) {
	h.LLMClient = client
	h.LLMModels = models
	if client != nil && provider != "" {
		h.LLMProviderName = provider
	}
}

// LLMProvider returns the LLM provider name (empty = no LLM).
func (h *Handler) LLMProvider() string {
	if h.LLMClient == nil {
		return ""
	}
	return h.LLMProviderName
}

// LLMModel returns the default model name.
func (h *Handler) LLMModel() string {
	if len(h.LLMModels) > 0 {
		return h.LLMModels[0]
	}
	return ""
}

// SetWriteChannels sets the write channels (call before starting ReadLoop).
func (h *Handler) SetWriteChannels(writeCh chan<- WriteMsg, writeDone <-chan struct{}) {
	h.ensureManagers()
	h.stdioMgr.SetWriteChannels(writeCh, writeDone)
}

// Cleanup releases all resources (stdio processes, background tasks).
func (h *Handler) Cleanup() {
	if h.stdioMgr != nil {
		h.stdioMgr.Cleanup()
	}
	if h.bgMgr != nil {
		h.bgMgr.Cleanup()
	}
}

// ensureManagers ensures the stdio and bg task managers are initialized.
func (h *Handler) ensureManagers() {
	if h.stdioMgr == nil {
		h.stdioMgr = newStdioManager(h.Verbose, h.dockerMode, h.LogFunc)
		h.stdioMgr.executor = h.Executor
	}
	if h.bgMgr == nil {
		ws := ""
		if h.PathGuard != nil {
			ws = h.PathGuard.Workspace
		}
		h.bgMgr = newBgTaskManager(h.Verbose, h.dockerMode, ws, h.LogFunc)
		h.bgMgr.executor = h.Executor
	}
}

// HandleRequest processes a request and returns a response.
func (h *Handler) HandleRequest(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	resp := h.Dispatch(msg)

	if resp.Type == runnerproto.ProtoError {
		var e runnerproto.ErrorResponse
		if json.Unmarshal(resp.Body, &e) == nil {
			callLogf(h.LogFunc, "← %s [id=%s] error: %s — %s", msg.Type, msg.ID, e.Code, e.Message)
		}
	} else if h.Verbose {
		callLogf(h.LogFunc, "← %s [id=%s] ok", msg.Type, msg.ID)
	}

	return resp
}

// Dispatch routes a message to the appropriate handler based on its type.
func (h *Handler) Dispatch(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	h.ensureManagers()

	switch msg.Type {
	case "exec":
		return h.handleExec(msg)
	case runnerproto.ProtoBgExec:
		return h.handleBgExec(msg)
	case runnerproto.ProtoBgKill:
		return h.handleBgKill(msg)
	case runnerproto.ProtoBgStatus:
		return h.handleBgStatus(msg)
	case runnerproto.ProtoLLMGenerate:
		return handleLLMGenerate(msg, h.LLMClient, h.LogFunc)
	case runnerproto.ProtoLLMModels:
		return handleLLMModels(msg, h.LLMClient, h.LLMModels, h.LogFunc)
	case "read_file":
		return h.handleReadFile(msg)
	case "write_file":
		return h.handleWriteFile(msg)
	case "stat":
		return h.handleStat(msg)
	case "read_dir":
		return h.handleReadDir(msg)
	case "mkdir_all":
		return h.handleMkdirAll(msg)
	case "remove":
		return h.handleRemove(msg)
	case "remove_all":
		return h.handleRemoveAll(msg)
	case "download_file":
		return h.handleDownloadFile(msg)
	case runnerproto.ProtoStdioStart:
		return h.stdioMgr.HandleStart(msg)
	case runnerproto.ProtoStdioClose:
		return h.stdioMgr.HandleClose(msg)
	default:
		return runnerproto.MakeError(msg.ID, "EINVAL", fmt.Sprintf("unknown request type: %s", msg.Type))
	}
}

// DispatchFireAndForget handles messages that don't need a response.
func (h *Handler) DispatchFireAndForget(msg runnerproto.RunnerMessage) {
	h.ensureManagers()

	switch msg.Type {
	case runnerproto.ProtoStdioWrite:
		h.stdioMgr.HandleWrite(msg)
	}
}

func (h *Handler) handleExec(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.ExecRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid exec request: "+err.Error())
	}

	timeout := time.Duration(req.Timeout) * time.Second
	// Guard against integer overflow: cap at 1 hour
	if req.Timeout <= 0 || req.Timeout > 3600 {
		timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	spec := ExecSpec{
		Command:   req.Command,
		Args:      req.Args,
		Shell:     req.Shell,
		Dir:       req.Dir,
		Env:       req.Env,
		Stdin:     req.Stdin,
		Timeout:   timeout,
		RunAsUser: req.RunAsUser,
	}

	// pathguard checks working directory
	if spec.Dir != "" && h.PathGuard != nil {
		if err := h.PathGuard.Validate(spec.Dir); err != nil {
			return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
		}
	}

	result, err := h.Executor.Exec(ctx, spec)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "exec error: "+err.Error())
	}

	callLogf(h.LogFunc, "  exec done  exit=%d  stdout=%dB  stderr=%dB", result.ExitCode, len(result.Stdout), len(result.Stderr))
	return runnerproto.MakeResponse(msg.ID, "exec_result", runnerproto.ExecResultResponse{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
		TimedOut: result.TimedOut,
	})
}

func (h *Handler) handleReadFile(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.ReadFileRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	data, err := h.Executor.ReadFile(path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  read_file %s (%d bytes)", req.Path, len(data))
	}
	return runnerproto.MakeResponse(msg.ID, "file_content", runnerproto.FileContentResponse{
		Data: base64.StdEncoding.EncodeToString(data),
	})
}

func (h *Handler) handleWriteFile(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.WriteFileRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	data, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid base64: "+err.Error())
	}
	if err := h.Executor.WriteFile(path, data, os.FileMode(req.Perm)); err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  write_file %s (%d bytes)", req.Path, len(data))
	}
	return runnerproto.MakeOK(msg.ID)
}

func (h *Handler) handleStat(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.StatRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	info, err := h.Executor.Stat(path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	return runnerproto.MakeResponse(msg.ID, "file_info", runnerproto.StatResponse{
		Name:    info.Name,
		Size:    info.Size,
		Mode:    uint32(info.Mode),
		ModTime: info.ModTime.Format(time.RFC3339),
		IsDir:   info.IsDir,
	})
}

func (h *Handler) handleReadDir(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.ReadDirRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	entries, err := h.Executor.ReadDir(path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	resp := runnerproto.DirEntriesResponse{Entries: make([]runnerproto.DirEntryResponse, 0, len(entries))}
	for _, e := range entries {
		resp.Entries = append(resp.Entries, runnerproto.DirEntryResponse{
			Name:  e.Name,
			IsDir: e.IsDir,
			Size:  e.Size,
		})
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  read_dir %s (%d entries)", req.Path, len(resp.Entries))
	}
	return runnerproto.MakeResponse(msg.ID, "dir_entries", resp)
}

func (h *Handler) handleMkdirAll(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.PathRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	if err := h.Executor.MkdirAll(path, os.FileMode(req.Perm)); err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  mkdir_all %s", req.Path)
	}
	return runnerproto.MakeOK(msg.ID)
}

func (h *Handler) handleRemove(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.PathRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	if err := h.Executor.Remove(path); err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  remove %s", req.Path)
	}
	return runnerproto.MakeOK(msg.ID)
}

func (h *Handler) handleRemoveAll(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.PathRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.Path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}
	if err := h.Executor.RemoveAll(path); err != nil {
		return runnerproto.MakeError(msg.ID, runnerproto.ProtoErrorCode(err), err.Error())
	}
	if h.Verbose {
		callLogf(h.LogFunc, "  remove_all %s", req.Path)
	}
	return runnerproto.MakeOK(msg.ID)
}

func (h *Handler) handleDownloadFile(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.DownloadFileRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", err.Error())
	}
	path, err := h.safePath(req.OutputPath)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
	}

	// 5-minute timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	size, err := h.Executor.DownloadFile(ctx, req.URL, path)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "download failed: "+err.Error())
	}

	callLogf(h.LogFunc, "  download_file %s → %s (%d bytes)", req.URL, req.OutputPath, size)
	return runnerproto.MakeResponse(msg.ID, runnerproto.ProtoOK, runnerproto.DownloadFileResponse{Size: size})
}

func (h *Handler) handleBgExec(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.BgExecRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid bg_exec request: "+err.Error())
	}

	// pathguard checks working directory
	if req.Dir != "" && h.PathGuard != nil {
		if err := h.PathGuard.Validate(req.Dir); err != nil {
			return runnerproto.MakeError(msg.ID, "EPERM", err.Error())
		}
	}

	resp, err := h.bgMgr.Start(req)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "bg_exec failed: "+err.Error())
	}

	return runnerproto.MakeResponse(msg.ID, runnerproto.ProtoBgStarted, resp)
}

func (h *Handler) handleBgKill(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.BgKillRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid bg_kill request: "+err.Error())
	}

	if err := h.bgMgr.Kill(req); err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "bg_kill failed: "+err.Error())
	}

	return runnerproto.MakeOK(msg.ID)
}

func (h *Handler) handleBgStatus(msg runnerproto.RunnerMessage) *runnerproto.RunnerMessage {
	var req runnerproto.BgStatusRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid bg_status request: "+err.Error())
	}

	resp, err := h.bgMgr.Status(req)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "bg_status failed: "+err.Error())
	}

	return runnerproto.MakeResponse(msg.ID, runnerproto.ProtoBgOutput, resp)
}

// safePath is a convenience method for PathGuard.SafePath.
func (h *Handler) safePath(path string) (string, error) {
	if h.PathGuard == nil {
		return path, nil
	}
	return h.PathGuard.SafePath(path)
}
