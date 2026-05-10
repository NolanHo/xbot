package agent

import (
	"context"
	"encoding/json"

	"xbot/bus"
	"xbot/channel"
	"xbot/protocol"
)

// Transport is the execution layer. Every Backend method goes through Transport.
//
// Local mode uses localTransport (in-process handler dispatch that directly
// operates on *Agent). Remote mode uses RemoteTransport (WebSocket RPC to xbot server).
//
// The key insight: Backend is a pure typed RPC client. Transport decides whether
// the call executes locally or remotely. Backend never branches on mode.
type Transport interface {
	// === Lifecycle ===
	Start(ctx context.Context) error
	Stop()
	Close() error
	Run(ctx context.Context) error // blocks until done (local: agent.Run, remote: <-ctx.Done())

	// === RPC ===
	// Call sends a request and returns the response.
	// method is an RPC method name (e.g. "get_settings").
	Call(method string, payload json.RawMessage) (json.RawMessage, error)

	// === Communication ===
	SendMessage(msg protocol.InboundMessage) error
	// BindChat registers a chat session for event routing (WS channel subscription).
	BindChat(chatID string) error

	// === Event subscription (new protocol-based API) ===
	// Subscribe registers a handler for protocol events matching the given pattern.
	// Returns a cancel function to unsubscribe.
	Subscribe(pattern protocol.EventPattern, handler protocol.EventHandler) (cancel func())

	// === Server-push events (Deprecated: use Subscribe instead) ===
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"outbound"}, handler) instead.
	OnOutbound(cb func(bus.OutboundMessage))
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"progress"}, handler) instead.
	OnProgress(cb func(*channel.CLIProgressPayload))
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"inject_user"}, handler) instead.
	OnInjectUserMessage(cb func(chatID, content string))
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"reconnect"}, handler) instead.
	OnReconnect(cb func())
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"conn_state"}, handler) instead.
	OnConnStateChange(cb func(state string))
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"plugin_widget"}, handler) instead.
	OnPluginWidgets(cb func(zones map[string]string, chatID string))
	// Deprecated: Use Subscribe(protocol.EventPattern{Type:"tui_control"}, handler) instead.
	OnTUIControlRequest(cb func(action string, params map[string]string) (map[string]string, error))

	// === State ===
	ConnState() string
	IsRemote() bool
	ServerURL() string
}
