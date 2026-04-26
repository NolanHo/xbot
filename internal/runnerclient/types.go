package runnerclient

import "time"

const (
	// PingPeriod is the interval between heartbeat pings
	PingPeriod = 30 * time.Second
	// PongWait is the timeout for waiting for a pong response
	PongWait = 60 * time.Second
	// WriteWait is the timeout for write operations
	WriteWait = 10 * time.Second
)

// WriteMsg is a message sent through the single writer goroutine.
type WriteMsg struct {
	Data []byte
	Err  chan error // non-nil means control message (e.g. ping) that needs error reporting
}

// LogFunc is the log callback function type.
type LogFunc func(format string, args ...interface{})

// callLogf safely calls the log function (nil-safe).
func callLogf(logf LogFunc, format string, args ...interface{}) {
	if logf != nil {
		logf(format, args...)
	}
}
