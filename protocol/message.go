package protocol

import "time"

type InboundMessage struct {
	From string `json:"from"`
	To   string `json:"to"`

	Channel    string `json:"channel"`
	SenderID   string `json:"sender_id"`
	SenderName string `json:"sender_name"`
	ChatID     string `json:"chat_id"`
	ChatType   string `json:"chat_type"`

	Content string   `json:"content"`
	Media   []string `json:"media,omitempty"`

	Metadata  map[string]string `json:"metadata,omitempty"`
	Time      time.Time         `json:"time"`
	IsCron    bool              `json:"is_cron,omitempty"`
	RequestID string            `json:"request_id,omitempty"`

	EventSource  string `json:"event_source,omitempty"`
	EventTrigger string `json:"event_trigger,omitempty"`

	ParentAgentID string          `json:"parent_agent_id,omitempty"`
	SystemPrompt  string          `json:"system_prompt,omitempty"`
	AllowedTools  []string        `json:"allowed_tools,omitempty"`
	RoleName      string          `json:"role_name,omitempty"`
	Capabilities  map[string]bool `json:"capabilities,omitempty"`
}

type OutboundMessage struct {
	From string `json:"from"`
	To   string `json:"to"`

	Channel string `json:"channel"`
	ChatID  string `json:"chat_id"`

	Content string   `json:"content"`
	Media   []string `json:"media,omitempty"`

	Metadata map[string]string `json:"metadata,omitempty"`

	IsPartial bool `json:"is_partial,omitempty"`

	ToolsUsed   []string `json:"tools_used,omitempty"`
	WaitingUser bool     `json:"waiting_user,omitempty"`
	Error       string   `json:"error,omitempty"`
}
