package agentapi

import (
	"context"
	"time"

	"xbot/protocol"
)

// SettingsClient 设置读写接口。2 方法。
type SettingsClient interface {
	GetSettings(namespace, senderID string) (map[string]string, error)
	SetSetting(namespace, senderID, key, value string) error
}

// LLMClient LLM 模型与运行时配置接口。13 方法。
type LLMClient interface {
	ListModels() []string
	GetDefaultModel() string
	GetContextMode() string
	SwitchModel(senderID, model string) error
	GetUserMaxOutputTokens(senderID string) int
	SetUserMaxOutputTokens(senderID string, maxTokens int) error
	GetUserThinkingMode(senderID string) string
	SetUserThinkingMode(senderID string, mode string) error
	GetLLMConcurrency(senderID string) int
	SetLLMConcurrency(senderID string, personal int) error
	SetContextMode(mode string) error
	SetDefaultThinkingMode(mode string) error
	SetModelTiers(cfg ModelTiersConfig) error
}

// SubscriptionClient LLM 订阅管理接口。8 方法。
type SubscriptionClient interface {
	ListSubscriptions(senderID string) ([]Subscription, error)
	AddSubscription(senderID string, sub Subscription) error
	RemoveSubscription(id string) error
	UpdateSubscription(id string, sub Subscription) error
	GetDefaultSubscription(senderID string) (*Subscription, error)
	SetDefaultSubscription(id string, chatID string) error
	RenameSubscription(id, name string) error
	SetSubscriptionModel(id, model string) error
}

// MemoryClient 会话记忆与历史管理接口。6 方法。
type MemoryClient interface {
	ClearMemory(ctx context.Context, channel, chatID, targetType, senderID string) error
	GetMemoryStats(ctx context.Context, channel, chatID, senderID string) map[string]string
	GetHistory(channel, chatID string) ([]HistoryMessage, error)
	TrimHistory(channel, chatID string, cutoff time.Time) error
	GetTokenState(channel, chatID string) (promptTokens, completionTokens int64, err error)
	ResetTokenState()
}

// SessionClient 会话运行时配置接口。4 方法。
type SessionClient interface {
	SetCWD(ch, chatID, dir string) error
	SetMaxIterations(n int)
	SetMaxConcurrency(n int)
	SetMaxContextTokens(n int)
}

// ProcessingClient 处理状态查询接口。3 方法。
type ProcessingClient interface {
	IsProcessing(ch, chatID string) bool
	GetActiveProgress(ch, chatID string) *protocol.ProgressEvent
	GetTodos(ch, chatID string) []TodoItem
}

// InteractiveClient 交互式子代理会话管理接口。6 方法。
type InteractiveClient interface {
	CountInteractiveSessions(channelName, chatID string) int
	ListInteractiveSessions(channelName, chatID string) []InteractiveSessionInfo
	InspectInteractiveSession(ctx context.Context, roleName, channelName, chatID, instance string, tailCount int) (string, error)
	GetSessionMessages(channelName, chatID, roleName, instance string) ([]SessionMessage, bool)
	GetAgentSessionDump(channelName, chatID, roleName, instance string) (*AgentSessionDump, bool)
	GetAgentSessionDumpByFullKey(fullKey string) (*AgentSessionDump, bool)
}

// BgTaskClient 后台任务管理接口。3 方法。
type BgTaskClient interface {
	ListBgTasks(sessionKey string) ([]BgTaskJSON, error)
	KillBgTask(taskID string) error
	CleanupCompletedBgTasks(sessionKey string)
}

// TenantClient 租户管理接口。1 方法。
type TenantClient interface {
	ListTenants() ([]TenantInfo, error)
}
