package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"xbot/bus"
	"xbot/channel"
	"xbot/config"
	llm "xbot/llm"
	"xbot/protocol"
	"xbot/tools"
)

// DirectBackend implements AgentBackend by directly calling *Agent methods.
// It is used in local (in-process) mode as a replacement for localTransport's
// RPC handler table. No JSON marshaling or RPC dispatch — straight Go calls.
type DirectBackend struct {
	ag            *Agent
	reconfigureFn func(channel string)
}

var _ AgentBackend = (*DirectBackend)(nil)

// NewDirectBackend creates a DirectBackend that delegates to the given Agent.
func NewDirectBackend(ag *Agent) *DirectBackend {
	return &DirectBackend{ag: ag}
}

// SetReconfigureFn sets the callback invoked after a channel config change.
func (d *DirectBackend) SetReconfigureFn(fn func(channel string)) {
	d.reconfigureFn = fn
}

// ---------------------------------------------------------------------------
// Lifecycle — managed by LifecycleManager, no-op here
// ---------------------------------------------------------------------------

func (d *DirectBackend) Start(_ context.Context) error { return nil }
func (d *DirectBackend) Stop()                         {}
func (d *DirectBackend) Close() error                  { return nil }
func (d *DirectBackend) Run(_ context.Context) error   { return nil }

// ---------------------------------------------------------------------------
// Transport identity
// ---------------------------------------------------------------------------

func (d *DirectBackend) IsRemote() bool    { return false }
func (d *DirectBackend) ConnState() string { return "connected" }
func (d *DirectBackend) ServerURL() string { return "" }

// ---------------------------------------------------------------------------
// LLM Management
// ---------------------------------------------------------------------------

func (d *DirectBackend) GetContextMode() string {
	return d.ag.GetContextMode()
}

func (d *DirectBackend) SetContextMode(mode string) error {
	return d.ag.SetContextMode(mode)
}

func (d *DirectBackend) SetUserModel(senderID, model string) error {
	return d.ag.SetUserModel(senderID, model)
}

func (d *DirectBackend) SwitchModel(senderID, model, chatID string) error {
	if chatID != "" {
		d.ag.LLMFactory().SwitchModel(senderID, model, chatID)
	} else {
		d.ag.LLMFactory().SwitchModel(senderID, model)
	}
	return nil
}

func (d *DirectBackend) GetUserMaxContext(senderID string) int {
	return d.ag.GetUserMaxContext(senderID)
}

func (d *DirectBackend) SetUserMaxContext(senderID string, maxContext int) error {
	return d.ag.SetUserMaxContext(senderID, maxContext)
}

func (d *DirectBackend) GetUserMaxOutputTokens(senderID string) int {
	return d.ag.GetUserMaxOutputTokens(senderID)
}

func (d *DirectBackend) SetUserMaxOutputTokens(senderID string, maxTokens int) error {
	if maxTokens < 0 {
		return fmt.Errorf("max_output_tokens must be >= 0, got %d", maxTokens)
	}
	if err := d.ag.SetUserMaxOutputTokens(senderID, maxTokens); err != nil {
		// Only fallback to factory-level setting when user has no DB config.
		if strings.Contains(err.Error(), "未配置自定义 LLM") {
			d.ag.LLMFactory().SetUserMaxOutputTokens(senderID, maxTokens)
			return nil
		}
		return err
	}
	return nil
}

func (d *DirectBackend) GetUserThinkingMode(senderID string) string {
	return d.ag.GetUserThinkingMode(senderID)
}

func (d *DirectBackend) SetUserThinkingMode(senderID string, mode string) error {
	validModes := map[string]bool{"": true, "enabled": true, "disabled": true, "auto": true}
	if !validModes[mode] {
		return fmt.Errorf("invalid thinking_mode: %q", mode)
	}
	if err := d.ag.SetUserThinkingMode(senderID, mode); err != nil {
		// Only fallback to factory-level setting when user has no DB config.
		if strings.Contains(err.Error(), "未配置自定义 LLM") {
			d.ag.LLMFactory().SetUserThinkingMode(senderID, mode)
			return nil
		}
		return err
	}
	return nil
}

func (d *DirectBackend) GetLLMConcurrency(senderID string) int {
	return d.ag.GetLLMConcurrency(senderID)
}

func (d *DirectBackend) SetLLMConcurrency(senderID string, personal int) error {
	return d.ag.SetLLMConcurrency(senderID, personal)
}

func (d *DirectBackend) ClearProxyLLM(senderID string) {
	d.ag.ClearProxyLLM(senderID)
}

func (d *DirectBackend) SetModelTiers(cfg config.LLMConfig) error {
	d.ag.LLMFactory().SetModelTiers(cfg)
	return nil
}

func (d *DirectBackend) SetDefaultThinkingMode(mode string) error {
	d.ag.LLMFactory().SetDefaultThinkingMode(mode)
	return nil
}

func (d *DirectBackend) SetModelContexts(contexts map[string]int) error {
	d.ag.LLMFactory().SetModelContexts(contexts)
	return nil
}

func (d *DirectBackend) SetGlobalMaxTokens(maxTokens int) error {
	d.ag.LLMFactory().SetGlobalMaxTokens(maxTokens)
	return nil
}

func (d *DirectBackend) SetRetryConfig(cfg llm.RetryConfig) error {
	d.ag.LLMFactory().SetRetryConfig(cfg)
	return nil
}

func (d *DirectBackend) SetChatLLM(chatID string, provider string, llmCfg config.LLMConfig) error {
	var inner llm.LLM
	switch provider {
	case "anthropic":
		inner = llm.NewAnthropicLLM(llm.AnthropicConfig{
			BaseURL:      llmCfg.BaseURL,
			APIKey:       llmCfg.APIKey,
			DefaultModel: llmCfg.Model,
			MaxTokens:    llmCfg.MaxOutputTokens,
		})
	default:
		inner = llm.NewOpenAILLM(llm.OpenAIConfig{
			BaseURL:      llmCfg.BaseURL,
			APIKey:       llmCfg.APIKey,
			DefaultModel: llmCfg.Model,
			MaxTokens:    llmCfg.MaxOutputTokens,
		})
	}
	client := llm.NewRetryLLM(inner, llm.DefaultRetryConfig())
	d.ag.LLMFactory().SetChatLLM("", chatID, client, llmCfg.Model)
	return nil
}

func (d *DirectBackend) SetSandbox(sb tools.Sandbox, mode string) {
	d.ag.SetSandbox(sb, mode)
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

func (d *DirectBackend) SetCWD(ch, chatID, dir string) error {
	if d.ag.sandboxMode != "none" {
		return fmt.Errorf("CWD sync not supported in %s sandbox mode", d.ag.sandboxMode)
	}
	if d.ag.MultiSession() == nil {
		return ErrNoSessionManager
	}
	sess, err := d.ag.MultiSession().GetOrCreateSession(ch, chatID)
	if err != nil {
		return err
	}
	// If session already has a persisted CWD (restored from disk), keep it.
	// Otherwise use the requested directory.
	if sess.GetCurrentDir() == "" {
		sess.SetCurrentDir(dir)
	}
	// Always refresh plugin contexts so script plugins see the correct workDir
	if d.ag.pluginMgr != nil {
		cwd := sess.GetCurrentDir()
		d.ag.pluginMgr.RefreshWorkDir(cwd, ch, chatID, sess.TenantID())
		d.ag.pluginMgr.RefreshTenantID(sess.TenantID())
	}
	return nil
}

func (d *DirectBackend) SetMaxIterations(n int)            { d.ag.SetMaxIterations(n) }
func (d *DirectBackend) SetMaxConcurrency(n int)           { d.ag.SetMaxConcurrency(n) }
func (d *DirectBackend) SetCompressionThreshold(f float64) { d.ag.SetCompressionThreshold(f) }

func (d *DirectBackend) SetMaxContextTokens(n int, chatID ...string) {
	d.ag.SetMaxContextTokens(n, chatID...)
}

func (d *DirectBackend) GetEffectiveMaxContext(senderID, chatID string) int {
	return d.ag.LLMFactory().GetEffectiveMaxContext(senderID, chatID)
}

func (d *DirectBackend) ClearPerChatMaxContext(chatID string) {
	d.ag.LLMFactory().ClearPerChatMaxContext(chatID)
}

func (d *DirectBackend) IsProcessing(ch, chatID string) bool {
	key := ch + ":" + chatID
	_, found := d.ag.chatCancelCh.Load(key)
	return found
}

func (d *DirectBackend) GetActiveProgress(ch, chatID string) *protocol.ProgressEvent {
	key := ch + ":" + chatID
	v, ok := d.ag.lastProgressSnapshot.Load(key)
	if !ok {
		return nil
	}
	snapshot := v.(*protocol.ProgressEvent)
	// Shallow copy to avoid data race: agent may update snapshot fields concurrently.
	result := *snapshot
	if histPtr, ok := d.ag.iterationHistories.Load(key); ok {
		hist := *histPtr.(*[]protocol.ProgressEvent)
		if len(hist) > 0 {
			result.IterationHistory = make([]protocol.ProgressEvent, len(hist))
			copy(result.IterationHistory, hist)
			return &result
		}
	}
	return &result
}

func (d *DirectBackend) GetTodos(ch, chatID string) []protocol.TodoItem {
	key := ch + ":" + chatID
	if d.ag.todoManager == nil {
		return []protocol.TodoItem{}
	}
	items := d.ag.todoManager.GetTodos(key)
	if len(items) == 0 {
		return []protocol.TodoItem{}
	}
	result := make([]protocol.TodoItem, len(items))
	for i, t := range items {
		result[i] = protocol.TodoItem{ID: t.ID, Text: t.Text, Done: t.Done}
	}
	return result
}

// ---------------------------------------------------------------------------
// History / Memory
// ---------------------------------------------------------------------------

func (d *DirectBackend) ResetTokenState() {
	// no-op in local mode (same as localTransport handler)
}

func (d *DirectBackend) GetHistory(ch, chatID string) ([]protocol.HistoryMessage, error) {
	ms := d.ag.MultiSession()
	if ms == nil {
		return nil, fmt.Errorf("multi-session not available")
	}
	sess, err := ms.GetOrCreateSession(ch, chatID)
	if err != nil {
		return nil, err
	}
	msgs, err := sess.GetMessages()
	if err != nil {
		return nil, err
	}
	return channel.ConvertMessagesToHistory(msgs), nil
}

func (d *DirectBackend) TrimHistory(ch, chatID string, cutoff time.Time) error {
	ms := d.ag.MultiSession()
	if ms == nil {
		return fmt.Errorf("multi-session not available")
	}
	return ms.TrimHistory(ch, chatID, cutoff)
}

// ---------------------------------------------------------------------------
// SubAgent / Interactive Sessions
// ---------------------------------------------------------------------------

func (d *DirectBackend) CountInteractiveSessions(ch, chatID string) int {
	return d.ag.CountInteractiveSessions(ch, chatID)
}

func (d *DirectBackend) ListInteractiveSessions(ch, chatID string) []InteractiveSessionInfo {
	return d.ag.ListInteractiveSessions(ch, chatID)
}

func (d *DirectBackend) InspectInteractiveSession(ctx context.Context, roleName, ch, chatID, instance string, tailCount int) (string, error) {
	return d.ag.InspectInteractiveSession(ctx, roleName, ch, chatID, instance, tailCount)
}

func (d *DirectBackend) GetSessionMessages(ch, chatID, roleName, instance string) ([]SessionMessage, bool) {
	return d.ag.GetSessionMessages(ch, chatID, roleName, instance)
}

func (d *DirectBackend) GetAgentSessionDump(ch, chatID, roleName, instance string) (*AgentSessionDump, bool) {
	return d.ag.GetAgentSessionDump(ch, chatID, roleName, instance)
}

func (d *DirectBackend) GetAgentSessionDumpByFullKey(fullKey string) (*AgentSessionDump, bool) {
	return d.ag.GetAgentSessionDumpByFullKey(fullKey)
}

// ---------------------------------------------------------------------------
// Background Tasks
// ---------------------------------------------------------------------------

func (d *DirectBackend) GetBgTaskCount(sessionKey string) int {
	if d.ag.bgTaskMgr == nil {
		return 0
	}
	return len(d.ag.bgTaskMgr.ListRunning(sessionKey))
}

func (d *DirectBackend) ListBgTasks(sessionKey string) ([]BgTaskJSON, error) {
	if d.ag.bgTaskMgr == nil {
		return nil, nil
	}
	tasks := d.ag.bgTaskMgr.ListAllForSession(sessionKey)
	result := make([]BgTaskJSON, len(tasks))
	for i, t := range tasks {
		result[i] = BgTaskJSON{
			ID: t.ID, Command: t.Command, Status: string(t.Status),
			StartedAt: t.StartedAt.Format(time.RFC3339), ExitCode: t.ExitCode,
			Output: t.Output, Error: t.Error,
		}
		if t.FinishedAt != nil {
			result[i].FinishedAt = t.FinishedAt.Format(time.RFC3339)
		}
	}
	return result, nil
}

func (d *DirectBackend) KillBgTask(taskID string) error {
	if d.ag.bgTaskMgr == nil {
		return ErrBgTasksUnavailable
	}
	return d.ag.bgTaskMgr.Kill(taskID)
}

func (d *DirectBackend) CleanupCompletedBgTasks(sessionKey string) {
	if d.ag.bgTaskMgr != nil {
		d.ag.bgTaskMgr.RemoveCompletedTasks(sessionKey)
	}
}

// ---------------------------------------------------------------------------
// Channel Config
// ---------------------------------------------------------------------------

func (d *DirectBackend) GetChannelConfigs() (map[string]map[string]string, error) {
	cfg := config.LoadFromFile(config.ConfigFilePath())
	if cfg == nil {
		return nil, fmt.Errorf("config not found")
	}
	result := make(map[string]map[string]string)
	result["web"] = map[string]string{
		"enabled": strconv.FormatBool(cfg.Web.Enable),
		"host":    cfg.Web.Host,
		"port":    strconv.Itoa(cfg.Web.Port),
	}
	result["feishu"] = map[string]string{
		"enabled":            strconv.FormatBool(cfg.Feishu.Enabled),
		"app_id":             cfg.Feishu.AppID,
		"app_secret":         cfg.Feishu.AppSecret,
		"encrypt_key":        cfg.Feishu.EncryptKey,
		"verification_token": cfg.Feishu.VerificationToken,
		"domain":             cfg.Feishu.Domain,
	}
	result["qq"] = map[string]string{
		"enabled":       strconv.FormatBool(cfg.QQ.Enabled),
		"app_id":        cfg.QQ.AppID,
		"client_secret": cfg.QQ.ClientSecret,
	}
	result["napcat"] = map[string]string{
		"enabled": strconv.FormatBool(cfg.NapCat.Enabled),
		"ws_url":  cfg.NapCat.WSUrl,
		"token":   cfg.NapCat.Token,
	}
	return result, nil
}

func (d *DirectBackend) SetChannelConfig(ch string, values map[string]string) error {
	cfg := config.LoadFromFile(config.ConfigFilePath())
	if cfg == nil {
		cfg = &config.Config{}
	}
	switch ch {
	case "web":
		if v, ok := values["enabled"]; ok {
			cfg.Web.Enable, _ = strconv.ParseBool(v)
		} else if v, ok := values["enable"]; ok {
			cfg.Web.Enable, _ = strconv.ParseBool(v)
		}
		if v, ok := values["host"]; ok {
			cfg.Web.Host = v
		}
		if v, ok := values["port"]; ok {
			cfg.Web.Port, _ = strconv.Atoi(v)
		}
	case "feishu":
		if v, ok := values["enabled"]; ok {
			cfg.Feishu.Enabled, _ = strconv.ParseBool(v)
		}
		if v, ok := values["app_id"]; ok {
			cfg.Feishu.AppID = v
		}
		if v, ok := values["app_secret"]; ok {
			cfg.Feishu.AppSecret = v
		}
		if v, ok := values["encrypt_key"]; ok {
			cfg.Feishu.EncryptKey = v
		}
		if v, ok := values["verification_token"]; ok {
			cfg.Feishu.VerificationToken = v
		}
		if v, ok := values["domain"]; ok {
			cfg.Feishu.Domain = v
		}
	case "qq":
		if v, ok := values["enabled"]; ok {
			cfg.QQ.Enabled, _ = strconv.ParseBool(v)
		}
		if v, ok := values["app_id"]; ok {
			cfg.QQ.AppID = v
		}
		if v, ok := values["client_secret"]; ok {
			cfg.QQ.ClientSecret = v
		}
	case "napcat":
		if v, ok := values["enabled"]; ok {
			cfg.NapCat.Enabled, _ = strconv.ParseBool(v)
		}
		if v, ok := values["ws_url"]; ok {
			cfg.NapCat.WSUrl = v
		}
		if v, ok := values["token"]; ok {
			cfg.NapCat.Token = v
		}
	default:
		return fmt.Errorf("unknown channel: %s", ch)
	}
	if err := config.SaveToFile(config.ConfigFilePath(), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	if d.reconfigureFn != nil {
		d.reconfigureFn(ch)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Panic stubs — these methods are never called through DirectBackend.
// They exist only to satisfy the AgentBackend interface.
// ---------------------------------------------------------------------------

func (d *DirectBackend) SendInbound(_ bus.InboundMessage) error {
	panic("not implemented in DirectBackend: SendInbound (use LifecycleManager)")
}

func (d *DirectBackend) Subscribe(_ protocol.EventPattern, _ protocol.EventHandler) func() {
	panic("not implemented in DirectBackend: Subscribe")
}

func (d *DirectBackend) BindChat(_ string) error {
	panic("not implemented in DirectBackend: BindChat")
}

func (d *DirectBackend) CallRPC(_ string, _ any) (json.RawMessage, error) {
	panic("not implemented in DirectBackend: CallRPC")
}

func (d *DirectBackend) WireCallbacks(
	_ func(msg bus.OutboundMessage) (string, error),
	_ func(name string) (channel.Channel, bool),
	_ func(ev protocol.SessionEvent),
	_ bus.MessageSender,
	_ func(name string, runFn bus.RunFn) error,
	_ func(name string),
) {
	panic("not implemented in DirectBackend: WireCallbacks (managed by LocalLifecycle)")
}

func (d *DirectBackend) SetChatRenameFn(_ func(chatID, newName string) (oldName string, err error)) {
	panic("not implemented in DirectBackend: SetChatRenameFn")
}

func (d *DirectBackend) SetTUIControlHandler(_ func(action string, params map[string]string) (map[string]string, error)) {
	panic("not implemented in DirectBackend: SetTUIControlHandler")
}

func (d *DirectBackend) GetSettings(_, _ string) (map[string]string, error) {
	panic("not implemented in DirectBackend: GetSettings (use h.Ag.SettingsService() directly)")
}

func (d *DirectBackend) SetSetting(_, _, _, _ string) error {
	panic("not implemented in DirectBackend: SetSetting (use h.Ag.SettingsService() directly)")
}

func (d *DirectBackend) ListModels() []string {
	panic("not implemented in DirectBackend: ListModels (use h.Ag.LLMFactory() directly)")
}

func (d *DirectBackend) ListAllModels() []string {
	panic("not implemented in DirectBackend: ListAllModels (use h.Ag.LLMFactory() directly)")
}

func (d *DirectBackend) GetDefaultModel() string {
	panic("not implemented in DirectBackend: GetDefaultModel (use h.Ag.LLMFactory() directly)")
}

func (d *DirectBackend) ClearMemory(_ context.Context, _, _, _, _ string) error {
	panic("not implemented in DirectBackend: ClearMemory (use h.Ag directly)")
}

func (d *DirectBackend) GetMemoryStats(_ context.Context, _, _, _ string) map[string]string {
	panic("not implemented in DirectBackend: GetMemoryStats (use h.Ag directly)")
}

func (d *DirectBackend) GetTokenState(_, _ string) (int64, int64, error) {
	panic("not implemented in DirectBackend: GetTokenState")
}

func (d *DirectBackend) GetUserTokenUsage(_ string) (map[string]any, error) {
	panic("not implemented in DirectBackend: GetUserTokenUsage")
}

func (d *DirectBackend) GetDailyTokenUsage(_ string, _ int) ([]map[string]any, error) {
	panic("not implemented in DirectBackend: GetDailyTokenUsage")
}

func (d *DirectBackend) ListSubscriptions(_ string) ([]protocol.Subscription, error) {
	panic("not implemented in DirectBackend: ListSubscriptions")
}

func (d *DirectBackend) GetDefaultSubscription(_ string) (*protocol.Subscription, error) {
	panic("not implemented in DirectBackend: GetDefaultSubscription")
}

func (d *DirectBackend) AddSubscription(_ string, _ protocol.Subscription) error {
	panic("not implemented in DirectBackend: AddSubscription")
}

func (d *DirectBackend) RemoveSubscription(_ string) error {
	panic("not implemented in DirectBackend: RemoveSubscription")
}

func (d *DirectBackend) SetDefaultSubscription(_, _ string) error {
	panic("not implemented in DirectBackend: SetDefaultSubscription")
}

func (d *DirectBackend) RenameSubscription(_, _ string) error {
	panic("not implemented in DirectBackend: RenameSubscription")
}

func (d *DirectBackend) UpdateSubscription(_ string, _ protocol.Subscription) error {
	panic("not implemented in DirectBackend: UpdateSubscription")
}

func (d *DirectBackend) UpdatePerModelConfig(_, _ string, _ protocol.PerModelConfig) error {
	panic("not implemented in DirectBackend: UpdatePerModelConfig")
}

func (d *DirectBackend) SetSubscriptionModel(_, _ string) error {
	panic("not implemented in DirectBackend: SetSubscriptionModel")
}

func (d *DirectBackend) ListTenants() ([]TenantInfo, error) {
	panic("not implemented in DirectBackend: ListTenants")
}

func (d *DirectBackend) RegisterCoreTool(_ tools.Tool) {
	panic("not implemented in DirectBackend: RegisterCoreTool")
}

func (d *DirectBackend) RegisterTool(_ tools.Tool) {
	panic("not implemented in DirectBackend: RegisterTool")
}

func (d *DirectBackend) IndexGlobalTools() {
	panic("not implemented in DirectBackend: IndexGlobalTools")
}

func (d *DirectBackend) CreateWebUser(_ string) (string, error) {
	panic("not implemented in DirectBackend: CreateWebUser")
}

func (d *DirectBackend) ListWebUsers() ([]map[string]any, error) {
	panic("not implemented in DirectBackend: ListWebUsers")
}

func (d *DirectBackend) DeleteWebUser(_ string) error {
	panic("not implemented in DirectBackend: DeleteWebUser")
}

func (d *DirectBackend) DeleteChat(_, _, _ string) error {
	panic("not implemented in DirectBackend: DeleteChat")
}

func (d *DirectBackend) RenameChat(_, _, _, _ string) error {
	panic("not implemented in DirectBackend: RenameChat")
}
