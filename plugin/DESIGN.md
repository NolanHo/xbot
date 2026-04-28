# Plugin System Design

## Overview

xbot 插件系统提供类似 VSCode 的可扩展性，允许第三方开发者通过统一的 Plugin API 扩展 xbot 的行为。

**设计哲学**: 统一 Plugin API + 多运行时支持。插件开发者只关心一套接口，运行时（native/gRPC/WASM）是透明的实现细节。

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  Plugin API (Go Interface)                │
│  Plugin · PluginManifest · PluginContext                  │
├──────────────┬───────────────────┬───────────────────────┤
│   Native     │     gRPC          │      WASM (Phase 2)   │
│  (in-process)│ (external process)│  (wazero sandbox)     │
├──────────────┴───────────────────┴───────────────────────┤
│                 PluginManager (lifecycle)                  │
├───────────────────────────────────────────────────────────┤
│  Integration Layer: Tool Registry · Hooks · Middleware     │
└───────────────────────────────────────────────────────────┘
```

## Core Concepts

### 1. Plugin Manifest (`plugin.json`)

每个插件目录下的声明式配置文件，描述插件的元信息、能力贡献、激活条件和权限需求。

```json
{
  "id": "com.example.code-reviewer",
  "name": "Code Reviewer",
  "version": "1.0.0",
  "description": "AI-powered code review tool",
  "author": "example.com",
  "runtime": "native",
  "entry": "main.go",
  "activationEvents": ["onTool:code_review", "onStart"],
  "permissions": ["tools.register", "hooks.subscribe", "storage.private", "network.outbound"],
  "contributes": {
    "tools": [
      {
        "name": "code_review",
        "description": "Review code changes and suggest improvements",
        "inputSchema": { ... }
      }
    ],
    "hooks": [
      { "event": "PostToolUse", "matcher": "Shell(*git commit*)" }
    ],
    "contextEnrichers": [
      { "name": "git_status", "description": "Inject current git status" }
    ]
  }
}
```

### 2. Plugin Interface

```go
type Plugin interface {
    // Manifest returns plugin metadata. Called once during discovery.
    Manifest() PluginManifest

    // Activate initializes the plugin and registers capabilities.
    // Called when an activation event fires.
    Activate(ctx PluginContext) error

    // Deactivate cleans up plugin resources. Called on shutdown.
    Deactivate(ctx PluginContext) error
}
```

### 3. PluginContext Interface

受限的能力接口，不暴露 ToolContext 原始字段：

```go
type PluginContext interface {
    // Tool registration
    RegisterTool(tool PluginTool) error

    // Hook subscription
    OnPreToolUse(matcher string, handler HookHandler) error
    OnPostToolUse(matcher string, handler HookHandler) error
    OnUserPrompt(handler HookHandler) error
    OnAgentStop(handler HookHandler) error

    // Context enrichment (upgrade Skills to executable)
    EnrichContext(name string, enricher ContextEnricher) error

    // Isolated storage (per-plugin namespace)
    Storage() StorageAccessor

    // Logging
    Logger() Logger

    // Metadata
    PluginID() string
    WorkingDir() string
    Channel() string
    ChatID() string
}
```

### 4. Runtime Types

| Runtime | Isolation | Latency | Language Support | Status |
|---------|-----------|---------|-----------------|--------|
| native  | Interface boundary | ~μs (zero-copy) | Go only | Phase 1 |
| gRPC    | Process isolation | ~1-5ms | Any (via protobuf) | Phase 1 |
| wasm    | Sandbox isolation | ~0.5-2ms | WASM-targeting languages | Phase 2 |

### 5. Plugin Lifecycle

```
Discovery → Load Manifest → [Wait for Activation Event]
  → Activate() → Register capabilities → [Running]
  → [Deactivation Event] → Deactivate() → Cleanup
```

Activation Events:
- `onStart` — xbot 启动时立即激活
- `onTool:<name>` — 首次调用指定工具时激活
- `onHook:<event>` — 首次触发指定钩子事件时激活
- `onCommand:<cmd>` — 用户输入指定命令时激活

### 6. Permission System

| Permission | Description |
|-----------|-------------|
| `tools.register` | 注册新工具 |
| `tools.call` | 调用其他工具 |
| `hooks.subscribe` | 订阅生命周期钩子 |
| `context.enrich` | 注入系统提示内容 |
| `storage.private` | 插件私有 KV 存储 |
| `storage.shared` | 跨插件共享存储 |
| `network.outbound` | 发起网络请求 |
| `bus.read` | 读取消息总线 |
| `bus.write` | 写入消息总线 |

### 7. Integration with Existing Systems

**Tool Registration**: PluginTool → adapter → tools.Tool → Registry.Register()

**Hook Subscription**: Plugin HookHandler → adapter → hooks.CallbackHook → Manager.RegisterBuiltin()

**Context Enrichment**: ContextEnricher → adapter → MessageMiddleware → Pipeline.Use()

**Storage**: ~/.xbot/plugins/<id>/storage.db (per-plugin isolated SQLite)

### 8. Plugin Tool V2 (ToolCallContext)

PluginToolV2 是 PluginTool 的向后兼容扩展，通过 `ToolCallContext` 传递丰富的会话信息：

```go
type ToolCallContext struct {
    SessionID string          // 当前会话 ID
    Channel   string          // 消息渠道（cli/feishu/web）
    ChatID    string          // 聊天 ID
    UserID    string          // 触发用户 ID
    Ctx       context.Context // 取消和超时控制
}

type PluginToolV2 interface {
    PluginTool
    ExecuteWithContext(ctx *ToolCallContext, input string) (*ToolResult, error)
}
```

**V2 检测策略**：`PluginToolAdapter` 通过 interface assertion 检测底层 tool 是否实现 `PluginToolV2`：
- V2 工具：调用 `ExecuteWithContext`，传入完整会话信息
- V1 工具：fallback 到 `Execute(ctx context.Context, input)`
- `SimplePluginTool` 同时实现两个接口，通过 `ExecV2Fn` 字段可选启用 V2

### 9. Health Check

可选的 `HealthChecker` 接口允许插件报告自身健康状态：

```go
type HealthChecker interface {
    HealthCheck(ctx context.Context) error
}

func (pm *PluginManager) HealthCheck(ctx context.Context) map[string]error
```

- 仅检查 `StateActive` 的插件
- 未实现 `HealthChecker` 的插件视为健康（返回 nil）
- 实现 `HealthChecker` 且返回 error 的插件视为不健康
- 用于监控面板、运维告警、自动重启等场景

### 10. Metrics

`PluginMetrics` 提供插件系统的聚合指标：

```go
type PluginMetrics struct {
    TotalPlugins   int `json:"totalPlugins"`   // 总插件数
    ActivePlugins  int `json:"activePlugins"`  // 活跃插件数
    TotalTools     int `json:"totalTools"`      // 注册的工具总数
    TotalHooks     int `json:"totalHooks"`      // 注册的钩子总数
    TotalEnrichers int `json:"totalEnrichers"`  // 注册的上下文增强器总数
}

func (pm *PluginManager) Metrics() PluginMetrics
```

- 仅统计 `StateActive` 插件的 tools/hooks/enrichers
- JSON 标签用于 API 输出和序列化
- 支持运维监控和仪表盘集成

## File Structure

```
plugin/
├── plugin.go           # Plugin interface, PluginManifest, PluginTool
├── context.go          # PluginContext interface + implementations
├── manager.go          # PluginManager (discovery, lifecycle, routing)
├── manifest.go         # Manifest parsing and validation
├── permissions.go      # Permission checker
├── storage.go          # Per-plugin KV storage
├── runtime_native.go   # Native (in-process) runtime
├── runtime_grpc.go     # gRPC external process runtime
├── adapter_tool.go     # PluginTool → tools.Tool adapter
├── adapter_hook.go     # HookHandler → hooks.CallbackHook adapter
├── adapter_middleware.go # ContextEnricher → MessageMiddleware adapter
├── plugin.proto        # gRPC service definition (for remote plugins)
└── plugin_test.go      # Tests
```

## Design Decisions

### Why not pure WASM?

Roundtable 结论（5/5 专家同意）：WASM 不适合作为 V1 主运行时。

1. **调试工具链不成熟**：无法 attach debugger，crash 只能拿到 trap，无 coredump
2. **ToolContext 序列化复杂**：45+ 字段包含 6 个 callback 字段无法跨边界传递
3. **开发者体验差**：TS 开发者需要学习 WASM 工具链，无法使用 npm 生态
4. **Go + WASM 生态不成熟**：WASI 支持不完整，wazero 尚未大规模验证

WASM 作为 Phase 2 引入，用于高频轻量 hook 的沙箱场景。

### Why PluginToolV2 with interface embedding (not a breaking change)?

通过 interface embedding (`PluginToolV2` 嵌入 `PluginTool`) 实现 V2 扩展，而非修改现有 `PluginTool` 接口：

1. **零破坏性**：所有现有 PluginTool 实现无需任何修改
2. **渐进式迁移**：插件开发者按需实现 V2，获得会话上下文
3. **运行时检测**：通过 `tool.(PluginToolV2)` 动态判断，零开销 fallback
4. **SimplePluginTool 兼容**：默认走 V1 fallback，设置 ExecV2Fn 即启用 V2

### Why unified Plugin API first?

Platform Engineer 的论点被全员接受：先统一内部扩展点（Tool + Hook + Middleware + Skill），再在 API 下方替换运行时。这确保：
- 插件开发者只学一套 API
- 运行时切换对插件透明
- 现有 Go 工具可渐进迁移到 Plugin 接口

### Why PluginContext instead of ToolContext?

ToolContext 有 45+ 字段，包含 SendFunc、InjectInbound、Registry 等高权限成员。直接暴露给第三方插件等于裸奔。PluginContext 是按权限过滤的安全子集。

## Future (Phase 2)

1. WASM runtime via wazero (lightweight sandbox for trusted env)
2. TypeScript SDK (via protobuf-generated client)
3. Python SDK
4. Plugin Marketplace (registry + install command)
