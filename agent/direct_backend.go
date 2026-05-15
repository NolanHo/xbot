package agent

import (
	"fmt"
	"strconv"

	"xbot/config"
	"xbot/protocol"
	"xbot/tools"
)

// DirectBackend implements RPCHandlerBackend by directly calling *Agent methods.
// It is used in local (in-process) mode as a replacement for localTransport's
// RPC handler table. No JSON marshaling or RPC dispatch — straight Go calls.
type DirectBackend struct {
	ag            *Agent
	reconfigureFn func(channel string)
}

var _ RPCHandlerBackend = (*DirectBackend)(nil)

// NewDirectBackend creates a DirectBackend that delegates to the given Agent.
func NewDirectBackend(ag *Agent) *DirectBackend {
	return &DirectBackend{ag: ag}
}

// SetReconfigureFn sets the callback invoked after a channel config change.
func (d *DirectBackend) SetReconfigureFn(fn func(channel string)) {
	d.reconfigureFn = fn
}

// ---------------------------------------------------------------------------
// RPCHandlerBackend — methods needed by RPCTable + setting handlers
// ---------------------------------------------------------------------------

func (d *DirectBackend) GetContextMode() string {
	return d.ag.GetContextMode()
}

func (d *DirectBackend) SetContextMode(mode string) error {
	return d.ag.SetContextMode(mode)
}

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

func (d *DirectBackend) SetSandbox(sb tools.Sandbox, mode string) {
	d.ag.SetSandbox(sb, mode)
}

func (d *DirectBackend) SetModelTiers(cfg config.LLMConfig) error {
	d.ag.LLMFactory().SetModelTiers(cfg)
	return nil
}

// ---------------------------------------------------------------------------
// Session status — access Agent unexported fields
// ---------------------------------------------------------------------------

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
// Channel config — uses reconfigureFn callback
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
