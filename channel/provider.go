package channel

import (
	"context"

	"xbot/bus"
)

// ChannelProvider 由插件实现，用于注册自定义 Channel。
// 插件在 Activate 阶段通过 PluginContext.RegisterChannelProvider() 注册，
// serverapp 会在 registerChannels 和动态启停路径中查找并调用。
type ChannelProvider interface {
	// Name 返回唯一 channel 标识符（如 "telegram"）。
	// 不能与内置 channel（feishu/qq/napcat/web）重名。
	Name() string

	// CreateChannel 根据配置创建 Channel 实例。
	// cfg 来自 config.json 的 channels.<name> 段（map[string]string）。
	// msgBus 用于发送 Inbound 消息到 Agent。
	CreateChannel(cfg map[string]string, msgBus *bus.MessageBus) (Channel, error)

	// ConfigSchema 返回此 channel 的配置字段定义。
	// TUI settings 面板使用此 schema 自动渲染配置 UI。
	// 返回空 slice 表示该 channel 无可配置项。
	ConfigSchema() []SettingDefinition

	// IsEnabled 检查配置是否启用此 channel。
	// cfg 来自 config.json 的 channels.<name> 段。
	IsEnabled(cfg map[string]string) bool
}

// ChannelHistoryProvider is an optional extension for Channels that can load
// conversation history from the platform (e.g., Telegram, Discord).
// If the Channel created by CreateChannel also implements this interface,
// xbot will call LoadHistory when sessions are restored.
type ChannelHistoryProvider interface {
	// LoadHistory loads recent messages for a chat session.
	// Returns messages in chronological order (oldest first).
	// limit=0 means "use reasonable default" (e.g., 50).
	LoadHistory(ctx context.Context, chatID string, limit int) ([]PlatformMessage, error)
}

// ChannelUpdateProvider is an optional extension for Channels that support
// updating/deleting previously sent messages (e.g., edit in Telegram/Discord).
// This enables streaming-like behavior where the agent's reply is updated in-place.
type ChannelUpdateProvider interface {
	// UpdateMessage updates the content of a previously sent message.
	// messageID is the value returned by Send().
	UpdateMessage(ctx context.Context, chatID, messageID, newContent string) error

	// DeleteMessage removes a previously sent message.
	DeleteMessage(ctx context.Context, chatID, messageID string) error
}

// ChannelMediaProvider is an optional extension for Channels that support
// uploading/sending media files (images, documents, audio, video).
type ChannelMediaProvider interface {
	// SendMedia sends a media file to the specified chat.
	// mediaType is one of: "image", "file", "audio", "video".
	// filePath is a local path or URL.
	// caption is optional text accompanying the media.
	SendMedia(ctx context.Context, chatID, mediaType, filePath, caption string) (string, error)
}

// PlatformMessage represents a single message loaded from the platform's history.
// This is distinct from protocol.HistoryMessage which is xbot's internal session history.
type PlatformMessage struct {
	MessageID  string            `json:"message_id"`
	SenderID   string            `json:"sender_id"`
	SenderName string            `json:"sender_name"`
	Content    string            `json:"content"`
	IsBot      bool              `json:"is_bot"`
	Time       string            `json:"time"` // RFC3339
	Media      []MediaAttachment `json:"media,omitempty"`
}

// MediaAttachment represents a media file attached to a message.
type MediaAttachment struct {
	Type     string `json:"type"`              // "image", "file", "audio", "video"
	URL      string `json:"url,omitempty"`     // download URL
	FileID   string `json:"file_id,omitempty"` // platform-specific file ID
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`
}
