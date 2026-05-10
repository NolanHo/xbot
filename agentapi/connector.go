package agentapi

import (
	"context"
	"encoding/json"
)

// RPCMethod 类型化 RPC 方法常量。替代 agent/req_types.go 中的 string 常量，
// 提供编译时类型安全的方法名传递。
type RPCMethod string

// Connector 传输层唯一需要实现的接口。单方法。
// Backend 将所有 RPC 调用委托给 Connector，由 Connector 决定本地执行还是远程转发。
type Connector interface {
	Call(ctx context.Context, method RPCMethod, payload json.RawMessage) (json.RawMessage, error)
}
