package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"xbot/bus"
	"xbot/channel"
	"xbot/protocol"
)

// ---------------------------------------------------------------------------
// Lifecycle interfaces — extracted from the old Transport interface
// ---------------------------------------------------------------------------

// AgentRunner manages the Agent's lifecycle (start, run, stop).
// Local mode: runs Agent.Run(ctx) in-process.
// Remote mode: no-op (agent runs on the server).
type AgentRunner interface {
	Start(ctx context.Context) error
	Stop()
	Run(ctx context.Context) error // blocks until done
}

// EventRouter handles bidirectional message/event routing.
// Local mode: routes through bus + eventCh → baseTransport.
// Remote mode: routes through WebSocket.
type EventRouter interface {
	SendMessage(msg bus.InboundMessage) error
	BindChat(chatID string) error
	Subscribe(pattern protocol.EventPattern, handler protocol.EventHandler) (cancel func())
	ConnState() string
	IsRemote() bool
	ServerURL() string
}

// CallbackRegistry manages callback injection for Agent ↔ Channel binding.
// Local mode: injects directly into Agent.
// Remote mode: injects into WS client.
type CallbackRegistry interface {
	SetTUIControlHandler(cb func(action string, params map[string]string) (map[string]string, error))
	WireCallbacks(
		directSend func(msg bus.OutboundMessage) (string, error),
		channelFinder func(name string) (channel.Channel, bool),
		sessionStateHandler func(ev protocol.SessionEvent),
		messageSender bus.MessageSender,
		registerAgentChannel func(name string, runFn bus.RunFn) error,
		unregisterAgentChannel func(name string),
	)
	SetChatRenameFn(fn func(chatID, newName string) (oldName string, err error))
}

// ---------------------------------------------------------------------------
// LocalLifecycle — combines AgentRunner + EventRouter + CallbackRegistry
// for local (in-process) mode.
// ---------------------------------------------------------------------------

// LocalLifecycle manages the in-process Agent lifecycle, event routing,
// and callbacks. It replaces both localTransport and channelTransport.
type LocalLifecycle struct {
	agent *Agent
	bus   *bus.MessageBus
	base  baseTransport // for Subscribe/emit

	// TUI events (nil in server mode — eventLoop is a no-op)
	eventCh chan protocol.WSMessage
	cliCh   *channel.ChannelCliChannel

	// TUI control
	tuiCtrlMu sync.Mutex
	tuiCtrlCb func(string, map[string]string) (map[string]string, error)

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// Once ensures WireCallbacks is only called once.
	wireOnce sync.Once
}

// NewLocalLifecycle creates a LocalLifecycle for the given agent.
// eventCh may be nil for server mode (no TUI event loop).
func NewLocalLifecycle(ag *Agent, msgBus *bus.MessageBus, eventCh chan protocol.WSMessage) *LocalLifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &LocalLifecycle{
		agent:   ag,
		bus:     msgBus,
		base:    newBaseTransport(),
		eventCh: eventCh,
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
}

// SetCliChannel sets the ChannelCliChannel reference for TUI control responses.
func (l *LocalLifecycle) SetCliChannel(cliCh *channel.ChannelCliChannel) {
	l.cliCh = cliCh
}

// --- AgentRunner ---

func (l *LocalLifecycle) Start(ctx context.Context) error {
	if l.eventCh != nil {
		go l.eventLoop()
	} else {
		close(l.done)
	}
	go l.agent.Run(ctx)
	return nil
}

func (l *LocalLifecycle) Stop() {
	l.cancel()
	_ = l.agent.Close()
}

func (l *LocalLifecycle) Run(ctx context.Context) error {
	return l.agent.Run(ctx)
}

// --- EventRouter ---

func (l *LocalLifecycle) SendMessage(msg bus.InboundMessage) error {
	if l.bus == nil {
		return fmt.Errorf("message bus not available")
	}
	l.bus.Inbound <- msg
	return nil
}

func (l *LocalLifecycle) BindChat(chatID string) error { return nil }

func (l *LocalLifecycle) Subscribe(pattern protocol.EventPattern, handler protocol.EventHandler) (cancel func()) {
	return l.base.Subscribe(pattern, handler)
}

func (l *LocalLifecycle) ConnState() string { return "connected" }
func (l *LocalLifecycle) IsRemote() bool    { return false }
func (l *LocalLifecycle) ServerURL() string { return "" }

// --- CallbackRegistry ---

func (l *LocalLifecycle) SetTUIControlHandler(cb func(action string, params map[string]string) (map[string]string, error)) {
	l.tuiCtrlMu.Lock()
	l.tuiCtrlCb = cb
	l.tuiCtrlMu.Unlock()
	l.agent.SetTUICallbacks(cb, nil, nil)
}

func (l *LocalLifecycle) WireCallbacks(
	directSend func(msg bus.OutboundMessage) (string, error),
	channelFinder func(name string) (channel.Channel, bool),
	sessionStateHandler func(ev protocol.SessionEvent),
	messageSender bus.MessageSender,
	registerAgentChannel func(name string, runFn bus.RunFn) error,
	unregisterAgentChannel func(name string),
) {
	l.wireOnce.Do(func() {
		l.agent.WireCallbacks(directSend, channelFinder, sessionStateHandler, messageSender, registerAgentChannel, unregisterAgentChannel)
	})
}

func (l *LocalLifecycle) SetChatRenameFn(fn func(chatID, newName string) (oldName string, err error)) {
	l.agent.SetChatRenameFn(fn)
}

// --- eventLoop: mirrors channelTransport.eventLoop ---

func (l *LocalLifecycle) eventLoop() {
	defer close(l.done)
	for {
		select {
		case wsMsg, ok := <-l.eventCh:
			if !ok {
				return
			}
			l.dispatchWSMessage(wsMsg)
		case <-l.ctx.Done():
			return
		}
	}
}

// dispatchWSMessage converts a WSMessage to the appropriate event type.
// Mirrors channelTransport.dispatchWSMessage.
func (l *LocalLifecycle) dispatchWSMessage(msg protocol.WSMessage) {
	switch msg.Type {
	case protocol.MsgTypeProgress:
		if msg.Progress != nil {
			l.base.emit(l.ctx, msg.Progress)
		}
	case protocol.MsgTypeText:
		l.base.emit(l.ctx, protocol.OutboundEvent{
			ChatID:  msg.ChatID,
			Channel: msg.Channel,
			Content: msg.Content,
		})
	case protocol.MsgTypeStreamContent:
		if msg.Progress != nil {
			l.base.emit(l.ctx, &protocol.ProgressEvent{
				ChatID:                 msg.ChatID,
				StreamContent:          msg.Progress.StreamContent,
				ReasoningStreamContent: msg.Progress.ReasoningStreamContent,
			})
		}
	case protocol.MsgTypeAskUser:
		var ev protocol.AskUserEvent
		if err := json.Unmarshal([]byte(msg.Content), &ev); err == nil {
			l.base.emit(l.ctx, ev)
		}
	case protocol.MsgTypeInjectUser:
		l.base.emit(l.ctx, protocol.InjectUserEvent{
			ChatID:  msg.ChatID,
			Content: msg.Content,
		})
	case protocol.MsgTypeSession:
		if msg.Session != nil {
			l.base.emit(l.ctx, *msg.Session)
		}
	case protocol.MsgTypePluginWidgets:
		var zones map[string]string
		if err := json.Unmarshal([]byte(msg.Content), &zones); err == nil {
			l.base.emit(l.ctx, protocol.PluginWidgetEvent{
				ChatID: msg.ChatID,
				Zones:  zones,
			})
		}
	case protocol.MsgTypeTUIControlReq:
		l.tuiCtrlMu.Lock()
		cb := l.tuiCtrlCb
		l.tuiCtrlMu.Unlock()
		if cb != nil && msg.TUIControl != nil {
			result, err := cb(msg.TUIControl.Action, msg.TUIControl.Params)
			resp := &protocol.TUIControlPayload{
				Action: msg.TUIControl.Action,
			}
			if err != nil {
				resp.Error = err.Error()
			} else {
				resp.Result = result
			}
			if l.cliCh != nil {
				l.cliCh.DeliverTUIResponse(msg.ID, resp)
			}
		}
	}
}

// Compile-time checks
var (
	_ AgentRunner      = (*LocalLifecycle)(nil)
	_ EventRouter      = (*LocalLifecycle)(nil)
	_ CallbackRegistry = (*LocalLifecycle)(nil)
)
