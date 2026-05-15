package agent

import (
	"context"
	"encoding/json"
)

// ChannelTransport is the in-process direct-connect Transport for local mode.
// It directly calls RPCTable.Dispatch with no network overhead.
//
// This is the simplest possible Transport: just a function pointer to
// the server-side RPCTable.Dispatch, wrapped to match the Transport interface.
type ChannelTransport struct {
	dispatch func(ctx context.Context, method string, payload json.RawMessage) (json.RawMessage, error)
}

// NewChannelTransport creates a ChannelTransport from a dispatch function.
// The dispatch function is typically serverapp.RPCTable.Dispatch.
//
// Example:
//
//	table := serverapp.BuildRPCTable(cfg, directBackend, ag, disp, msgBus)
//	transport := agent.NewChannelTransport(table.Dispatch)
func NewChannelTransport(dispatch func(ctx context.Context, method string, payload json.RawMessage) (json.RawMessage, error)) *ChannelTransport {
	return &ChannelTransport{dispatch: dispatch}
}

// Call dispatches the RPC request directly in-process.
func (t *ChannelTransport) Call(method string, payload json.RawMessage) (json.RawMessage, error) {
	// Use a default context with no auth (local mode — always admin).
	// Per-request context is embedded in the RPCTable handlers via closures.
	ctx := context.Background()
	return t.dispatch(ctx, method, payload)
}

// Close is a no-op for in-process transport.
func (t *ChannelTransport) Close() error { return nil }

// Compile-time check.
var _ Transport = (*ChannelTransport)(nil)
