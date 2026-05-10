package agentapi

import "context"

// Lifecycle 生命周期管理接口。
type Lifecycle interface {
	Start(ctx context.Context) error
	Stop()
	Close() error
	Run(ctx context.Context) error
}
