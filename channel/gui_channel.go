package channel

import (
	"context"
	"sync"

	"github.com/google/uuid"
	log "xbot/logger"
)

// GUIMessageHandler is called by the GUIChannel to deliver outbound messages to the GUI frontend.
type GUIMessageHandler func(msg OutboundMsg)

// GUIInboundHandler is set by the channel to receive inbound messages from the frontend.
type GUIInboundHandler func(msg InboundMsg) error

// GUIChannel implements Channel for desktop GUI (Wails-based).
//
// Architecture:
//
//	┌─ Wails Frontend ─┐    ┌─ Go Backend ───────────────────┐
//	│  user input       │ →  │  GUIChannel.Send (InboundMsg)   │
//	│                    │    │      ↓                          │
//	│  render response   │ ←  │  GUIMessageHandler (OutboundMsg) │
//	└───────────────────┘    └─────────────────────────────────┘
type GUIChannel struct {
	name       string
	bus        interface{} // xbot's MessageBus (set by backend)
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex

	// onOutbound is called to deliver agent responses to the frontend.
	onOutbound GUIMessageHandler

	// handleInbound is called to process incoming user messages.
	handleInbound GUIInboundHandler

	// inboundCh delivers user messages from the frontend to the processing loop.
	inboundCh chan InboundMsg
}

// NewGUIChannel creates a new GUI channel.
func NewGUIChannel(name string) *GUIChannel {
	ctx, cancel := context.WithCancel(context.Background())
	return &GUIChannel{
		name:      name,
		ctx:       ctx,
		cancel:    cancel,
		inboundCh: make(chan InboundMsg, 100),
	}
}

// SetOutboundHandler sets the callback for delivering agent responses to the frontend.
func (g *GUIChannel) SetOutboundHandler(handler GUIMessageHandler) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.onOutbound = handler
}

// SetInboundHandler sets the callback for processing user messages from the frontend.
func (g *GUIChannel) SetInboundHandler(handler GUIInboundHandler) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.handleInbound = handler
}

// Name returns the channel name.
func (g *GUIChannel) Name() string {
	return g.name
}

// Start begins the channel's message processing loop.
func (g *GUIChannel) Start() error {
	log.Info("GUI channel starting: " + g.name)
	go g.processLoop()
	return nil
}

// Stop shuts down the channel.
func (g *GUIChannel) Stop() {
	log.Info("GUI channel stopping: " + g.name)
	g.cancel()
}

// Send implements Channel.Send — delivers an OutboundMsg from the agent to the GUI.
// The message is forwarded to the frontend via the onOutbound handler.
func (g *GUIChannel) Send(msg OutboundMsg) (string, error) {
	// Ensure message has an ID if empty
	if msg.ChatID == "" {
		msg.ChatID = g.name
	}

	// For messages with content, deliver to frontend via handler
	g.mu.RLock()
	handler := g.onOutbound
	g.mu.RUnlock()

	if handler != nil {
		handler(msg)
	}

	return "", nil
}

// PostInbound posts a user message from the frontend into the channel
// for processing by the backend agent.
func (g *GUIChannel) PostInbound(msg InboundMsg) error {
	msg.Channel = g.name
	if msg.RequestID == "" {
		msg.RequestID = uuid.New().String()
	}
	if msg.ChatID == "" {
		msg.ChatID = g.name
	}

	select {
	case g.inboundCh <- msg:
		return nil
	case <-g.ctx.Done():
		return g.ctx.Err()
	default:
		return nil
	}
}

// processLoop is the internal message processing loop.
// Incoming user messages are forwarded to the handleInbound callback.
func (g *GUIChannel) processLoop() {
	defer log.Info("GUI channel process loop exited: " + g.name)
	for {
		select {
		case <-g.ctx.Done():
			return
		case msg := <-g.inboundCh:
			log.Infof("GUI channel received: %s", msg.SenderID)

			g.mu.RLock()
			handler := g.handleInbound
			g.mu.RUnlock()

			if handler != nil {
				if err := handler(msg); err != nil {
					log.WithError(err).Warn("GUI inbound handler error")
				}
			}
		}
	}
}
