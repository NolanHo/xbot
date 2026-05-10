package agentapi

// FullBackend 通过结构体嵌入组合所有子接口。
// 实现者只需将各子接口的实例赋值给对应字段，
// FullBackend 即自动满足所有子接口的组合。
//
// Connector 字段可选：本地模式有 Connector，远程模式可能为 nil。
type FullBackend struct {
	Lifecycle
	SettingsClient
	LLMClient
	SubscriptionClient
	MemoryClient
	SessionClient
	ProcessingClient
	InteractiveClient
	BgTaskClient
	TenantClient
	// Connector 可选，本地模式提供
	Connector
}
