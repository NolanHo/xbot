package protocol

type ToolCallSnapshot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Args    string `json:"args"`
	Status  string `json:"status"`
	Elapsed int64  `json:"elapsed"`
}

type ProgressEvent struct {
	Iteration   int                `json:"iteration"`
	Content     string             `json:"content,omitempty"`
	Reasoning   string             `json:"reasoning,omitempty"`
	ToolCalls   []ToolCallSnapshot `json:"tool_calls,omitempty"`
	ElapsedWall int64              `json:"elapsed_wall"`
}

func (ProgressEvent) EventType() string { return "progress" }
func (ProgressEvent) EventVersion() int { return 1 }

type OutboundEvent struct {
	ChatID    string `json:"chat_id"`
	Content   string `json:"content"`
	IsPartial bool   `json:"is_partial"`
}

func (OutboundEvent) EventType() string { return "outbound" }
func (OutboundEvent) EventVersion() int { return 1 }

type InjectUserEvent struct {
	ChatID  string `json:"chat_id"`
	Content string `json:"content"`
}

func (InjectUserEvent) EventType() string { return "inject_user" }
func (InjectUserEvent) EventVersion() int { return 1 }

type ConnStateEvent struct {
	State string `json:"state"`
}

func (ConnStateEvent) EventType() string { return "conn_state" }
func (ConnStateEvent) EventVersion() int { return 1 }

type ReconnectEvent struct{}

func (ReconnectEvent) EventType() string { return "reconnect" }
func (ReconnectEvent) EventVersion() int { return 1 }

type PluginWidgetEvent struct {
	ChatID string            `json:"chat_id"`
	Zones  map[string]string `json:"zones"`
}

func (PluginWidgetEvent) EventType() string { return "plugin_widget" }
func (PluginWidgetEvent) EventVersion() int { return 1 }

type TUIControlEvent struct {
	Action  string                                    `json:"action"`
	Params  map[string]string                         `json:"params"`
	Respond func(result map[string]string, err error) `json:"-"`
}

func (TUIControlEvent) EventType() string { return "tui_control" }
func (TUIControlEvent) EventVersion() int { return 1 }
