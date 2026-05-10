# 万用事件系统 + 版本协商设计

## 目标

1. **永不改接口**：加新事件类型只需定义 struct + 两行方法，Transport/Backend/Channel 接口签名零改动
2. **版本协商**：不同版本的 transport 连接时自动协商到共同支持的版本，前向兼容
3. **本地零拷贝**：本地模式不因统一接口而退化（零序列化，纳秒级分发）
4. **编译期类型安全**：消费者通过泛型 wrapper 获得具体类型，无 `switch type` 散落

---

## 一、核心类型定义

### 1.1 TransportEvent — 自描述事件接口

```go
// protocol/event.go — 零依赖（仅 stdlib）

package protocol

// TransportEvent 是所有跨层事件的统一接口。
// 新事件类型 = 新 struct + 实现此接口的两个方法。Transport 接口不改。
type TransportEvent interface {
    EventType()    string  // 事件类型名，全局唯一。如 "progress", "message.outbound"
    EventVersion() int     // schema 版本。加字段不改号，删/改语义才递增
}
```

### 1.2 EventEnvelope — 纯序列化容器（单一路径）

```go
// protocol/envelope.go

import "encoding/json"

// EventEnvelope 是所有事件的统一容器。local 和 remote 共用同一结构。
// 不再区分 TypedPayload/SerializedPayload——统一走 JSON 序列化。
// 代价：本地模式多一次 json.Marshal（~2µs），换来零分支、零双路径。
type EventEnvelope struct {
    Type    string          `json:"type"`
    Version int             `json:"version"`
    Payload json.RawMessage `json:"payload"`
}
```

### 1.3 事件类型定义（示例）

```go
// protocol/events.go

// ProgressEvent — 流式进度（每 token 或每工具调用触发）
type ProgressEvent struct {
    Iteration   int    `json:"iteration"`
    Content     string `json:"content"`
    Reasoning   string `json:"reasoning,omitempty"`
    ToolCalls   []ToolCallSnapshot `json:"tool_calls,omitempty"`
    ElapsedWall int64  `json:"elapsed_wall_ms"`
}
func (ProgressEvent) EventType() string  { return "progress" }
func (ProgressEvent) EventVersion() int  { return 1 }

// OutboundEvent — 出站消息（agent 回复、stream delta）
type OutboundEvent struct {
    ChatID    string `json:"chat_id"`
    Content   string `json:"content"`
    IsPartial bool   `json:"is_partial"`
}
func (OutboundEvent) EventType() string  { return "message.outbound" }
func (OutboundEvent) EventVersion() int  { return 1 }

// InjectUserEvent — 注入用户消息（bg task 触发）
type InjectUserEvent struct {
    ChatID  string `json:"chat_id"`
    Content string `json:"content"`
}
func (InjectUserEvent) EventType() string  { return "message.inject" }
func (InjectUserEvent) EventVersion() int  { return 1 }

// ConnStateEvent — 连接状态变更
type ConnStateEvent struct {
    State string `json:"state"` // "connected" | "disconnected" | "reconnecting"
}
func (ConnStateEvent) EventType() string  { return "connection.state" }
func (ConnStateEvent) EventVersion() int  { return 1 }

// ReconnectEvent — 重连成功（无 payload）
type ReconnectEvent struct{}
func (ReconnectEvent) EventType() string  { return "connection.reconnect" }
func (ReconnectEvent) EventVersion() int  { return 1 }

// PluginWidgetEvent — 插件 UI 区域更新
type PluginWidgetEvent struct {
    ChatID string            `json:"chat_id"`
    Zones  map[string]string `json:"zones"`
}
func (PluginWidgetEvent) EventType() string  { return "plugin.widgets" }
func (PluginWidgetEvent) EventVersion() int  { return 1 }

// TUIControlEvent — 服务端 TUI 控制请求（请求-响应模式）
type TUIControlEvent struct {
    Action  string            `json:"action"`
    Params  map[string]string `json:"params"`
    Respond func(result map[string]string, err error) `json:"-"` // 本地专用
}
func (TUIControlEvent) EventType() string  { return "tui.control" }
func (TUIControlEvent) EventVersion() int  { return 1 }
```

---

## 二、Transport 接口 — 万用 Subscribe

```go
// transport/transport.go

type Transport interface {
    // === 生命周期 ===
    Start(ctx context.Context) error
    Stop()
    Close() error
    Run(ctx context.Context) error

    // === RPC ===
    Call(method string, payload json.RawMessage) (json.RawMessage, error)

    // === 消息 ===
    SendMessage(msg Message) error

    // === 事件（万用 callback）===
    //
    // Subscribe 是唯一的事件注册入口。返回 cancel() 取消订阅。
    // 加 ProgressEvent、OutboundEvent 或未来任何新事件，此签名永不变。
    Subscribe(pattern EventPattern, handler EventHandler) (cancel func())

    // === 状态 ===
    ConnState() string
    IsRemote() bool
    ServerURL() string
    NegotiatedVersion() int // 协商后的协议版本
}

// EventHandler 原始回调签名。建议消费者使用 TypedOn[T] 而非直接调用 Subscribe。
type EventHandler func(ctx context.Context, env EventEnvelope)

// EventPattern 订阅匹配规则
type EventPattern struct {
    Type       string // 事件类型名。"" = 通配所有
    MinVersion int    // 含，0 = 不设下限
    MaxVersion int    // 含，0 = 不设上限（即最新版本）
}
```

**对比现状：原来 7 个 OnXxx 方法 → 1 个 Subscribe。加事件类型 = 0 接口改动。**

| 现状 (7 个方法) | 新设计 (1 个方法) |
|---|---|
| `OnOutbound(func(bus.OutboundMessage))` | `Subscribe({Type:"message.outbound"}, handler)` |
| `OnProgress(func(*channel.CLIProgressPayload))` | `Subscribe({Type:"progress"}, handler)` |
| `OnInjectUserMessage(func(chatID, content))` | `Subscribe({Type:"message.inject"}, handler)` |
| `OnReconnect(func())` | `Subscribe({Type:"connection.reconnect"}, handler)` |
| `OnConnStateChange(func(state))` | `Subscribe({Type:"connection.state"}, handler)` |
| `OnPluginWidgets(func(zones, chatID))` | `Subscribe({Type:"plugin.widgets"}, handler)` |
| `OnTUIControlRequest(func(action, params) (map, error))` | `Subscribe({Type:"tui.control"}, handler)` |
| **未来加新事件 → 改接口** | **未来加新事件 → 不改接口** ✅ |

---

## 三、编译期类型安全 — TypedOn[T] 泛型包装（单一路径）

消费者**永远不直接调 Subscribe**，而是用类型安全的包装函数。local 和 remote 走完全相同的代码路径：

```go
// transport/typed.go

// TypedOn 提供编译期类型安全的订阅。local/remote 统一逻辑，无分支。
// 用法: TypedOn[ProgressEvent](transport, myHandler)
func TypedOn[T protocol.TransportEvent](t Transport, handler func(ctx context.Context, ev T)) (cancel func()) {
    var zero T
    eventType := zero.EventType()

    return t.Subscribe(EventPattern{Type: eventType}, func(ctx context.Context, env EventEnvelope) {
        var event T
        if err := json.Unmarshal(env.Payload, &event); err != nil {
            return
        }
        handler(ctx, event)
    })
}

// TypedOnVersion 带版本范围约束
func TypedOnVersion[T protocol.TransportEvent](t Transport, minVer, maxVer int, handler func(ctx context.Context, ev T)) (cancel func()) {
    var zero T
    return t.Subscribe(EventPattern{
        Type:       zero.EventType(),
        MinVersion: minVer,
        MaxVersion: maxVer,
    }, func(ctx context.Context, env EventEnvelope) {
        var event T
        json.Unmarshal(env.Payload, &event)
        handler(ctx, event)
    })
}
```

**消费者代码示例（local/remote 完全一致）：**

```go
cancel1 := TypedOn[ProgressEvent](transport, func(ctx context.Context, ev ProgressEvent) {
    ui.RenderProgress(ev.Iteration, ev.Content)
})

cancel2 := TypedOn[ConnStateEvent](transport, func(ctx context.Context, ev ConnStateEvent) {
    if ev.State == "disconnected" { startReconnect() }
})

cancel3 := TypedOnVersion[ProgressEvent](transport, 2, 3, handler)
cancel1() // 取消订阅
```

---

## 四、统一实现 — baseTransport（local 和 remote 共用）

核心洞察：**emit / dispatch / Subscribe / unsubscribe 逻辑完全通用**，local 和 remote 的唯一差异是事件来源和去向。抽到 baseTransport，两端嵌入复用。

```go
// transport/base.go

type subscription struct {
    pattern EventPattern
    handler EventHandler
}

// baseTransport 提供所有 Transport 的事件系统实现。
// localTransport 和 remoteTransport 通过嵌入复用它，零重复代码。
type baseTransport struct {
    mu           sync.RWMutex
    subs         map[string][]subscription // key=event type, O(1) 查找
    wildcardSubs []subscription            // Type="" 的订阅
}

func newBaseTransport() baseTransport {
    return baseTransport{
        subs: make(map[string][]subscription),
    }
}

// ========== Subscribe（唯一注册入口，两端完全共用） ==========

func (t *baseTransport) Subscribe(pattern EventPattern, handler EventHandler) (cancel func()) {
    t.mu.Lock()
    defer t.mu.Unlock()

    sub := subscription{pattern: pattern, handler: handler}
    if pattern.Type == "" {
        t.wildcardSubs = append(t.wildcardSubs, sub)
    } else {
        t.subs[pattern.Type] = append(t.subs[pattern.Type], sub)
    }

    var once sync.Once
    return func() { once.Do(func() { t.unsubscribe(sub) }) }
}

func (t *baseTransport) unsubscribe(sub subscription) {
    t.mu.Lock()
    defer t.mu.Unlock()
    // ... 从 subs 或 wildcardSubs 中移除
}

// ========== emit（发送端通用） ==========

// emit 序列化 TransportEvent → EventEnvelope → dispatch。
// local 和 remote 发送端都走此方法。单一路径，无分支。
func (t *baseTransport) emit(ctx context.Context, event protocol.TransportEvent) {
    payload, err := json.Marshal(event)
    if err != nil {
        return
    }
    env := EventEnvelope{
        Type:    event.EventType(),
        Version: event.EventVersion(),
        Payload: payload,
    }
    t.dispatch(ctx, env)
}

// ========== dispatch（分发端通用） ==========

// dispatch 按订阅表分发事件。O(1) 按 event type hash 查找。
// local 和 remote 接收端都走此方法。单一路径，无分支。
func (t *baseTransport) dispatch(ctx context.Context, env EventEnvelope) {
    t.mu.RLock()
    defer t.mu.RUnlock()

    // 精确匹配：按 event type O(1) 查找
    for _, sub := range t.subs[env.Type] {
        if sub.pattern.matches(env.Type, env.Version) {
            sub.handler(ctx, env)
        }
    }

    // wildcard 订阅：数量极少，直接遍历
    for _, sub := range t.wildcardSubs {
        if sub.pattern.matches(env.Type, env.Version) {
            sub.handler(ctx, env)
        }
    }
}
```

### 4.1 localTransport — 嵌入 baseTransport

```go
// transport/local.go

type localTransport struct {
    baseTransport // 嵌入，继承全部事件逻辑

    agent *Agent
    bus   *bus.MessageBus
}

func newLocalTransport(agent *Agent, bus *bus.MessageBus) *localTransport {
    return &localTransport{
        baseTransport: newBaseTransport(),
        agent:         agent,
        bus:           bus,
    }
}

// 发布事件：直接调 baseTransport.emit()
func (t *localTransport) notifyProgress(p *ProgressPayload) {
    t.emit(context.Background(), ProgressEvent{
        Iteration:   p.Iteration,
        Content:     p.ContentText,
        ToolCalls:   p.ToolCalls,
        ElapsedWall: p.Elapsed,
    })
}
```

### 4.2 remoteTransport — 嵌入 baseTransport + 网络收发

```go
// transport/remote.go

type remoteTransport struct {
    baseTransport // 嵌入，继承 dispatch/Subscribe

    conn        *websocket.Conn
    negEvents   map[string]int // event type → negotiated version
}

// 发送端：序列化后写 wire（和 baseTransport.emit 一样先 marshal，多加一步写网络）
func (t *remoteTransport) emitToWire(ctx context.Context, event protocol.TransportEvent) {
    payload, _ := json.Marshal(event)
    env := EventEnvelope{
        Type:    event.EventType(),
        Version: event.EventVersion(),
        Payload: payload,
    }
    t.conn.WriteJSON(env) // 走网络，对端 readPump 接收后调 dispatch
}

// 接收端（readPump）：从 wire 读到 EventEnvelope → 直接调 baseTransport.dispatch()
func (t *remoteTransport) handleWireMessage(raw []byte) {
    var env EventEnvelope
    if err := json.Unmarshal(raw, &env); err != nil {
        return
    }
    t.dispatch(context.Background(), env)
}
```

### 4.3 代码归属一览

```
baseTransport（transport/base.go）         ← 唯一的实现，两端复用
  ├── Subscribe()                         ← SubscribeOn/Off 逻辑在此一处
  ├── emit()                              ← 序列化+分发逻辑在此一处
  ├── dispatch()                          ← 订阅表查找+回调在此一处
  └── unsubscribe()

localTransport（transport/local.go）       ← 嵌入 baseTransport
  └── notifyXxx()                         ← 包装 emit() 调用，语义化命名

remoteTransport（transport/remote.go）     ← 嵌入 baseTransport
  ├── emitToWire()                        ← emit() + conn.WriteJSON()
  └── handleWireMessage()                 ← 读 wire → dispatch()

TypedOn[T]（transport/typed.go）          ← 泛型包装，永远只做 json.Unmarshal
```

**加新事件类型需要改的地方：**
1. `protocol/events.go` — 定义 struct（发布者和消费者都依赖）
2. 发布点 — 调 `emit()` 或 `emitToWire()`
3. 订阅点 — `TypedOn[T](...)`
4. **baseTransport — 0 改动** ✅
5. **Transport 接口 — 0 改动** ✅

---

## 五、版本协商 — Handshake 协议

仅在 remoteTransport 连接建立时执行一次。local 模式跳过（本地版本始终是最新的）。

```
Client                              Server
  |                                    |
  |--- WS Connect -------------------->|
  |                                    |
  |--- {"type":"handshake",            |
  |     "capabilities": [              |
  |       {"event":"progress","vers":[1,2]},
  |       {"event":"message.outbound","vers":[1]},
  |       ...
  |     ]} --------------------------->|
  |                                    |  取交集，协商版本
  |<-- {"type":"handshake_ack",       |
  |     "negotiated": [               |
  |       {"event":"progress","ver":2},
  |       {"event":"message.outbound","ver":1},
  |       ...
  |     ]} ---------------------------|
  |                                    |
  | 后续所有事件走协商版本              |
```

```go
// transport/version.go

type EventCapability struct {
    EventType string `json:"event"`
    Versions  []int  `json:"versions"` // 支持的版本列表（最高优先）
}

// Negotiate 取双方能力交集。关键事件无交集则拒绝连接。
func Negotiate(local, remote []EventCapability) ([]EventCapability, error) {
    remoteMap := make(map[string]map[int]bool, len(remote))
    for _, c := range remote {
        vers := make(map[int]bool, len(c.Versions))
        for _, v := range c.Versions {
            vers[v] = true
        }
        remoteMap[c.EventType] = vers
    }

    required := []string{"progress", "message.outbound"}
    negotiated := make([]EventCapability, 0, len(local))

    for _, c := range local {
        remoteVers, ok := remoteMap[c.EventType]
        if !ok {
            continue // 对端不支持此事件，可选事件允许缺失
        }
        highestCommon := 0
        for _, v := range c.Versions {
            if remoteVers[v] && v > highestCommon {
                highestCommon = v
            }
        }
        if highestCommon > 0 {
            negotiated = append(negotiated, EventCapability{
                EventType: c.EventType,
                Versions:  []int{highestCommon},
            })
        }
    }

    for _, req := range required {
        if !containsEvent(negotiated, req) {
            return nil, fmt.Errorf("version negotiation failed: peer doesn't support required event %q", req)
        }
    }
    return negotiated, nil
}
```

---

## 六、版本兼容矩阵

| 场景 | 本地版本 | 远程版本 | 协商结果 | 行为 |
|------|:----:|:----:|:----:|------|
| 同版本 | v2 | v2 | v2 | 全功能 |
| Client 旧 | v1 | v2 | v1 | 远程降级发送 v1 schema 事件 |
| Server 旧 | v2 | v1 | v1 | 本地降级发送 v1 schema 事件 |
| 部分事件不同版本 | progress v2, widget v1 | progress v1, widget v2 | progress v1, widget v1 | 各事件独立协商 |
| 可选事件缺失 | 支持 plugin.widgets | 不支持 | 跳过该事件 | 消费者收不到该类型事件 |
| 关键事件无交集 | 需要 progress | 无 progress | **拒绝连接** | 错误消息含缺失事件名 |

**前向兼容规则：**
- 加字段：版本号不变（老版本 json.Unmarshal 忽略未知字段）
- 删字段/改语义：版本号递增，老版本 consumer 通过 `TypedOnVersion` 限制版本范围

---

## 七、加一个新事件类型 — 完整流程

假设要加 `FileUploadEvent`（用户上传文件通知）：

### Step 1：定义事件 struct（2 行样板）

```go
// protocol/events.go

type FileUploadEvent struct {
    ChatID   string `json:"chat_id"`
    FileName string `json:"file_name"`
    FileSize int64  `json:"file_size"`
}
func (FileUploadEvent) EventType() string  { return "file.upload" }
func (FileUploadEvent) EventVersion() int  { return 1 }
```

### Step 2：发布者 emit

```go
// agent/... 或 channel/... 中

t.emit(ctx, FileUploadEvent{ChatID: chatID, FileName: name, FileSize: size})
```

### Step 3：消费者订阅（如需处理）

```go
cancel := TypedOn[FileUploadEvent](transport, func(ctx context.Context, ev FileUploadEvent) {
    ui.ShowNotification("新文件: " + ev.FileName)
})
```

### 改动总结

| 改了什么 | 行数 |
|---------|:---:|
| `protocol/events.go` — 加 struct + 2 方法 | +8 |
| 发布者 — emit 调用 | +3 |
| 消费者 — TypedOn 订阅（可选） | +4 |
| **Transport 接口** | **0** ✅ |
| **Backend 接口** | **0** ✅ |
| **Channel 接口** | **0** ✅ |
| **localTransport** | **0** ✅ |
| **RemoteTransport** | **0** ✅ |

---

## 八、迁移路径

### Phase 1：接口共存期

```go
// agent/transport.go — 过渡期同时支持旧 On* 和新 Subscribe

type Transport interface {
    // ... 其他方法 ...

    // === 新事件系统 ===
    Subscribe(pattern EventPattern, handler EventHandler) (cancel func())

    // === 旧回调（Deprecated） ===
    // Deprecated: use Subscribe({Type:"message.outbound"}, ...)
    OnOutbound(cb func(bus.OutboundMessage))
    // Deprecated: use Subscribe({Type:"progress"}, ...)
    OnProgress(cb func(*channel.CLIProgressPayload))
    // ... 其余 On* 标记 Deprecated
}
```

实现中，`OnProgress` 内部转为调用 `Subscribe({Type:"progress"}, adapter)`。

### Phase 2：消费者迁移

CLI 和 server 逐步从 `transport.OnProgress(cb)` 迁移到 `TypedOn[ProgressEvent](transport, handler)`。

### Phase 3：清理

删除所有 `On*` 方法和旧回调代码。

---

## 九、性能特性（统一序列化路径）

**设计决策：local 和 remote 统一走 `json.Marshal` + `json.Unmarshal`。local 的额外开销是一次序列化（~2µs），换来零双路径维护。**

| 路径 | 操作 | 分配 | 延迟 | 备注 |
|------|------|:---:|:---:|------|
| emit（local/remote 共用） | `json.Marshal(event)` → dispatch | 1 alloc | ~2µs | baseTransport.emit() |
| dispatch（local/remote 共用） | map lookup + handler 调用 | 0 alloc | ~50ns | baseTransport.dispatch() |
| TypedOn[T]（local/remote 共用） | `json.Unmarshal(payload)` | 1 alloc | ~2µs | 消费者泛型包装 |
| 远程额外 | WS write / read | — | ~100µs | 网络延迟为主 |

**单次事件端到端（emit → consumer handler）：**
- 本地：~4µs（一次 marshal + 一次 unmarshal）
- 远程：~104µs（同上 + 网络）

**高频场景（per-token progress，每秒 ~100 次）：**
- 本地 CPU 开销：~0.4ms/s（完全可以接受）
- 对比旧方案（零拷贝）：差异 < 0.4ms/s CPU

**结论：为消除双路径维护成本而接受的性能代价，在高频场景下也完全在可接受范围内。**

---

## 十、与三大包的集成

```
protocol/                          # 零依赖
  event.go                         # TransportEvent 接口 + EventEnvelope
  events.go                        # 所有事件类型定义
  version.go                       # EventCapability + Negotiate()

transport/                         # 仅依赖 protocol
  transport.go                     # Transport 接口 + Subscribe + EventPattern
  typed.go                         # TypedOn[T] 泛型包装
  local.go                         # 本地实现（零拷贝）
  remote.go                        # 远程实现（handshake + 序列化）
  version.go                       # Negotiate() 调用

agent/                             # 仅依赖 protocol + transport
  (emit 侧)                        # transport.emit(ctx, SomeEvent{...})

channel/                           # 仅依赖 protocol
  (消费侧)                         # TypedOn[ProgressEvent](transport, handler)
```

**编译器验证：** `channel/` 不 import `agent/`；`agent/` 不 import `channel/`；`transport/` 不 import `channel/` 或 `agent/`（除了 `Agent` 用于 localTransport 的 handler table）。

---

## 变更记录

| 日期 | 版本 | 作者 | 说明 |
|------|:----:|------|------|
| 2026-05-10 | v1.0 | 圆桌会议综合 | 初始设计，综合 Protocol Designer + Performance Pragmatist 方案 |
