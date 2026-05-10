package agentapi

import (
	"context"
	"encoding/json"
	"sync"

	"xbot/protocol"
)

type subscriber struct {
	pattern protocol.EventPattern
	handler protocol.EventHandler
}

// EventBus 基于缓冲 channel 的事件总线。
// 支持按 EventPattern 订阅和发布 protocol.TransportEvent。
type EventBus struct {
	ch          chan protocol.EventEnvelope
	subscribers []subscriber
	mu          sync.RWMutex
}

// NewEventBus 创建带指定缓冲大小的事件总线。
func NewEventBus(bufSize int) *EventBus {
	return &EventBus{
		ch: make(chan protocol.EventEnvelope, bufSize),
	}
}

// SubscribeEvent 按模式订阅事件。handler 在 Run loop 中同步调用。
func (b *EventBus) SubscribeEvent(pattern protocol.EventPattern, handler protocol.EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, subscriber{pattern, handler})
}

// SubscribeProgress 便捷方法：订阅 ProgressEvent，自动反序列化后回调。
func (b *EventBus) SubscribeProgress(handler func(protocol.ProgressEvent)) {
	b.SubscribeEvent(protocol.EventPattern{Type: "progress"}, func(e protocol.EventEnvelope) {
		var pe protocol.ProgressEvent
		if err := json.Unmarshal(e.Payload, &pe); err != nil {
			return
		}
		handler(pe)
	})
}

// Publish 发布事件到总线的缓冲 channel。非阻塞，缓冲区满时丢弃。
func (b *EventBus) Publish(event protocol.TransportEvent) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}
	env := protocol.EventEnvelope{
		Type:    event.EventType(),
		Version: event.EventVersion(),
		Payload: payload,
	}
	select {
	case b.ch <- env:
	default:
		// 缓冲区满，丢弃事件（非阻塞）
	}
}

// Run 启动事件分发循环。应在独立 goroutine 中调用。
// ctx 取消时 Run 退出。
func (b *EventBus) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-b.ch:
			b.dispatch(env)
		}
	}
}

func (b *EventBus) dispatch(env protocol.EventEnvelope) {
	b.mu.RLock()
	// 快照当前订阅者，避免持锁回调导致死锁
	subs := make([]subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()

	for _, s := range subs {
		if s.pattern.Matches(env.Type, env.Version) {
			s.handler(env)
		}
	}
}
