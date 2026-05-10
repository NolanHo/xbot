package agentapi

import "time"

// Subscription 表示 LLM 订阅配置。替代 channel.Subscription。
type Subscription struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	Model           string `json:"model"`
	MaxOutputTokens int    `json:"max_output_tokens"`
	ThinkingMode    string `json:"thinking_mode"`
	Active          bool   `json:"active"`
}

// HistoryMessage 表示会话中的一条历史消息。替代 channel.HistoryMessage。
type HistoryMessage struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// TodoItem 表示 TODO 列表中的一项。替代 channel.CLITodoItem。
type TodoItem struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// InteractiveSessionInfo 表示交互式子代理会话的快照。替代 agent.InteractiveSessionInfo。
type InteractiveSessionInfo struct {
	Role       string `json:"role"`
	Instance   string `json:"instance"`
	Running    bool   `json:"running"`
	Background bool   `json:"background"`
	Task       string `json:"task,omitempty"`
	Preview    string `json:"preview,omitempty"`
	ChatID     string `json:"chat_id"`
}

// SessionMessage 表示子代理对话中的单条消息。替代 agent.SessionMessage。
type SessionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// IterationToolSnapshot 捕获单次工具执行的结果。替代 agent.IterationToolSnapshot。
type IterationToolSnapshot struct {
	Name      string `json:"name"`
	Label     string `json:"label,omitempty"`
	Status    string `json:"status"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// IterationSnapshot 捕获已完成迭代的工具摘要。替代 agent.IterationSnapshot。
type IterationSnapshot struct {
	Iteration int                     `json:"iteration"`
	Thinking  string                  `json:"thinking,omitempty"`
	Reasoning string                  `json:"reasoning,omitempty"`
	Tools     []IterationToolSnapshot `json:"tools"`
}

// AgentSessionDump 包含交互式子代理会话的完整状态。替代 agent.AgentSessionDump。
type AgentSessionDump struct {
	Messages         []SessionMessage    `json:"messages"`
	IterationHistory []IterationSnapshot `json:"iterations"`
}

// BgTaskJSON 后台任务的 JSON 序列化摘要。替代 agent.BgTaskJSON。
type BgTaskJSON struct {
	ID         string `json:"id"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
	Output     string `json:"output"`
	ExitCode   int    `json:"exit_code"`
	Error      string `json:"error,omitempty"`
}

// TenantInfo 租户会话摘要。替代 agent.TenantInfo。
type TenantInfo struct {
	ID           int64  `json:"id"`
	Channel      string `json:"channel"`
	ChatID       string `json:"chat_id"`
	CreatedAt    string `json:"created_at"`
	LastActiveAt string `json:"last_active_at"`
}

// ModelTiersConfig 模型层级配置。替代 config.LLMConfig 中与 tier 相关的字段。
type ModelTiersConfig struct {
	Provider        string `json:"provider"`
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	Model           string `json:"model"`
	VanguardModel   string `json:"vanguard_model,omitempty"`
	BalanceModel    string `json:"balance_model,omitempty"`
	SwiftModel      string `json:"swift_model,omitempty"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
	ThinkingMode    string `json:"thinking_mode,omitempty"`
}
