# 计划：Settings/Subscription 系统完整重构

> 生成时间：2026-05-13
> 状态：待确认

## 背景与目标

### 问题根源

1. **设置写入路径分裂**：一个 Ctrl+S 触发 2-3 条独立写入路径（`saveSettings`、`ApplySettings`→`updateActiveSubscription`、`applyCLISettingsToBackend`），彼此不知道对方改了什么
2. **SubscriptionManager 有 3 套实现**：`configSubscriptionManager`（legacy config.json）、`localSubscriptionManager`（DB via Backend）、`remoteSubscriptionManager`（RPC via Backend），逻辑重复
3. **SettingHandler 有 2 份**：`serverapp/setting_handlers.go` 和 `cmd/xbot-cli/setting_handlers.go` 完全对称但独立维护
4. **`cmd/xbot-cli/main.go` 3043 行**：24 个 `IsRemote()` 分支、535 行 CLIChannelConfig 回调闭包

### 目标

1. **设置系统**：单一写入路径，每个 key 只有一个写入目标
2. **SubscriptionManager**：合并为 1 套（统一走 Backend 接口）
3. **SettingHandler**：合并为 1 份（去掉 CLI 端的重复）
4. **main.go**：从 3043 行降到 ≤1500 行，消除所有 IsRemote 分支（除了本质不同的 WS 连接逻辑）

## 现状分析

### 关键文件
| 文件 | 行数 | 职责 | 修改类型 |
|------|------|------|----------|
| `cmd/xbot-cli/main.go` | 3043 | CLI 入口，含 24 个 IsRemote 分支 | 大幅精简 |
| `cmd/xbot-cli/setting_handlers.go` | 239 | CLI 端设置 handler（和 server 端对称） | **删除** |
| `serverapp/setting_handlers.go` | 239 | Server 端设置 handler | 保留并增强 |
| `channel/cli_settings.go` | 195 | 设置面板读写 | 保留，小改 |
| `channel/cli_panel.go` | 3634 | 面板 UI（含 sub panel 保存） | 保留，sub panel 保存逻辑改 |
| `channel/cli_helpers.go` | 1851 | TUI 辅助函数 | 保留 |
| `serverapp/rpc_table.go` | 1248 | RPC handler（含 updateSubscription） | 保留，保护逻辑已修 |

### 依赖关系

```
用户 Ctrl+S
  → cli_settings.go:saveSettings()         ← PerModelConfig 写入
  → cli_settings.go → ApplySettings(values) → main.go 闭包
       → updateActiveSubscription()         ← 订阅字段写入
       → applyCLISettingsToConfig()         ← config 更新
       → applyCLISettingsToBackend()        ← 运行时生效
```

### 风险点
- **main.go 3043 行**：任何大范围重构都可能引入回归
- **`GetCurrentValues` 闭包 112 行**：Local 和 Remote 数据源完全不同
- **`SessionsList` 闭包 102 行**：两套完全独立的会话列表逻辑

## 详细计划

### 阶段一：合并 SubscriptionManager（删除 local/config 两套）

**目标**：3 套 SubscriptionManager → 1 套（统一走 Backend RPC/方法）

- [ ] 1.1 **删除 `configSubscriptionManager`**（main.go 2694-2825，132 行）
  - Local 模式也通过 Backend 接口操作订阅（Backend 在 local 模式下走 localTransport → sqlite）
  - `syncLLMFromActiveSub` 改为从 Backend.GetDefaultSubscription 读取
  - `saveCLIConfig` 不再写 LLM credentials 到 config.json

- [ ] 1.2 **删除 `localSubscriptionManager`**（main.go 2619-2663，45 行）
  - 已被 `remoteSubscriptionManager` 语义覆盖（都调 Backend 方法）
  - 统一使用 `remoteSubscriptionManager`（重命名为 `backendSubscriptionManager`）

- [ ] 1.3 **删除 `localLLMSubscriber`**（main.go 2665-2691，27 行）
  - 统一使用 `remoteLLMSubscriber`（重命名为 `backendLLMSubscriber`）

- [ ] 1.4 **删除种子订阅相关函数**（main.go 294-427，~134 行）
  - `localSeedSourceSubscriptions` / `hasActiveSeedSubscription` / `seedSubscriptionsForSender`
  - `seedLocalDBSubscriptionsFromConfig` / `loadLLMFromLocalDB` / `seedLocalDBSubscriptions` / `loadLLMFromDBSubscription`
  - 这些函数在 server 端已有 migration 逻辑处理
  - Local 模式首次启动时让 Backend 自动 seed（在 Backend 构造函数中处理）

**预期减少**：~338 行

### 阶段二：合并 SettingHandler（删除 CLI 端重复）

**目标**：2 份 SettingHandler → 1 份（只保留 serverapp 版）

- [ ] 2.1 **删除 `cmd/xbot-cli/setting_handlers.go`**（239 行）
  - CLI 端的 `applyCLISettingsToConfig` 和 `applyCLISettingsToBackend` 直接在 `ApplySettings` 闭包中处理
  - Local 模式：通过 Backend RPC → localTransport → Agent 方法
  - Remote 模式：通过 Backend RPC → WS → server 端 setting_handlers
  - **所有运行时效果都通过 Backend 接口触发**，不需要 CLI 端单独的 handler map

- [ ] 2.2 **改写 `ApplySettings` 闭包**（main.go 1278-1394，117 行 → ~40 行）
  - 删除 `isCLISubscriptionSettingKey` 和 `updateActiveSubscription` 调用
  - 统一为：`for k, v := range values { backend.SetSetting("cli", senderID, k, v) }`
  - Backend.SetSetting 在 local 模式下走 localTransport → Agent.SettingsService + applyRuntimeSetting
  - Backend.SetSetting 在 remote 模式下走 WS RPC → server 端 set_setting handler
  - LLM client 重建通过 Backend 的 `SetModelTiers` 等 RPC 方法处理

**预期减少**：~316 行

### 阶段三：统一 GetCurrentValues（消除最大闭包中的重复）

**目标**：`GetCurrentValues` 闭包从 112 行 → ~30 行

- [ ] 3.1 **在 Backend 接口上增加 `GetCurrentSettings` 方法**
  - 返回 `map[string]string`（合并 config defaults + DB settings + subscription fields）
  - Local 模式：localTransport handler 从 config + DB + subscription 读
  - Remote 模式：WS RPC `get_settings` 已实现（rpc_table.go:133-183）

- [ ] 3.2 **改写 `GetCurrentValues` 闭包**
  - 简化为：`return app.backend.GetCurrentSettings("cli", cliSenderID)`
  - 删除 `refreshRemoteValuesCache` 相关代码（main.go 113-238，~126 行）

- [ ] 3.3 **删除 `refreshRemoteValuesCache` 方法**（main.go 113-238，126 行）
  - 改为在需要时直接调 `backend.GetCurrentSettings()`

**预期减少**：~208 行

### 阶段四：提取初始化方法（简化 main()）

**目标**：main() 从 ~1650 行降到 ~800 行

- [ ] 4.1 **提取 `initLocalCLI(app, cliCh, cliCfg, db, ...)`**（~85 行）
  - 包含：local SettingsService、ModelLister、HistoryLoader、TUI callbacks、主题初始化
  - 来自 main.go 2014-2160, 2525-2545

- [ ] 4.2 **提取 `initRemoteCLI(app, cliCh, cliCfg, initialChatID)`**（~350 行）
  - 包含：WS 连接重试、事件订阅（7种）、RestoreSession、agent/plugin cache
  - 来自 main.go 1770-1788, 2190-2538

- [ ] 4.3 **改写 main() 核心流程为**
  ```go
  switch {
  case app.backend.IsRemote():
      app.initRemoteCLI(cliCh, cliCfg, initialChatID)
  default:
      app.initLocalCLI(cliCh, cliCfg, db, tenantSvc, ...)
  }
  ```

- [ ] 4.4 **统一 WebUser 三个闭包**（43 行 → ~15 行）
  - 通过 Backend 接口方法 `CreateWebUser`/`ListWebUsers`/`DeleteWebUser`

- [ ] 4.5 **统一 UsageQuery/AgentCount/AgentList 闭包**（77 行 → ~30 行）
  - 通过 Backend 接口方法，不再用 map 中转

**预期减少**：~510 行

### 阶段五：设置写入路径最终统一

**目标**：一个 Ctrl+S 只有一条写入路径

- [ ] 5.1 **改写 `cli_settings.go:saveSettings`**
  - PerModelConfigs：只通过 `subscriptionMgr.UpdatePerModelConfig(subID, model, pmc)` 新方法
  - 其他 subscription 字段：通过 `backend.SetSetting(key, value)` → 服务端统一处理
  - User-scoped 字段：通过 `backend.SetSetting(key, value)`
  - **不再调 `ApplySettings` 闭包**：SetSetting 已包含运行时生效

- [ ] 5.2 **在 Backend 接口增加 `UpdatePerModelConfig` 方法**
  - 只更新单个 PerModelConfig 条目，不碰其他字段
  - 服务端 RPC handler 只做 `existing.PerModelConfigs[model] = pmc; Update()`

- [ ] 5.3 **改写 sub panel 保存逻辑**（cli_panel.go 2727-2755）
  - 也使用 `UpdatePerModelConfig` 而不是整个 Subscription 对象

- [ ] 5.4 **删除 `ApplySettings` 闭包中的 subscription 写入逻辑**
  - 不再需要 `updateActiveSubscription` 函数（451-574 行，124 行）

**预期减少**：~124 行

## 验证方案

每个阶段完成后：
- `go build ./...` 编译通过
- `go test ./...` 测试通过
- `golangci-lint run ./...` 无新 warning
- **功能验证**（手动）：
  - `/settings` 改 max_context → 重新打开 → 值保持
  - sub panel 改 PerModelConfig override → 保存 → 重新打开 → 值保持
  - `/settings` 改主题/语言 → UI 立即生效
  - 新建会话 → 切换会话 → 设置正确继承
  - remote 模式 Ctrl+S 不丢失 API key

## 回滚策略

每个阶段独立提交 git commit，出问题可逐阶段 revert。
重构在 `fix/setup-no-repeat` 分支进行，不影响 main。

## 注意事项

- **不做**：Backend Transport 层重构（localTransport/RemoteTransport 已设计良好，只是 CLI 侧的调用方式需要统一）
- **不做**：server 端重构（serverapp/ 已经是 clean 的）
- **不做**：UI 层重构（cli_panel.go, cli_view.go 不动）
- **核心原则**：所有 CLI 侧操作都通过 Backend 接口 → 消除 IsRemote 分支

## 预期总体效果

| 指标 | 重构前 | 重构后 |
|------|--------|--------|
| main.go 行数 | 3043 | ~1500 |
| IsRemote() 调用 | 24 | ~3（只保留 WS 连接/事件订阅） |
| SubscriptionManager 实现 | 3 套 | 1 套 |
| SettingHandler | 2 份 | 1 份 |
| 设置写入路径 | 3-7 条 | 1 条 |
| Credential 丢失风险 | 高（masked key 写回） | 无（细粒度 API 不传凭证） |
