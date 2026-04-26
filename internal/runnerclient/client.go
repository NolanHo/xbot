package runnerclient

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/gorilla/websocket"

	"xbot/internal/runnerproto"
)

// ConnectOptions holds connection options.
type ConnectOptions struct {
	LLMProvider string  // LLM provider; empty = no LLM
	LLMModel    string  // default model name
	LogFunc     LogFunc // log callback (silent when nil)
}

// Connect establishes a WebSocket connection and sends a registration message.
func Connect(serverURL, userID, authToken, workspace, shell string, opts ...ConnectOptions) (*websocket.Conn, error) {
	var opt ConnectOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}

	callLogf(opt.LogFunc, "Dialing server %s ...", u.String())
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("dial server: %w", err)
	}

	// Reset read timeout on server heartbeat
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(PongWait))
		return nil
	})
	conn.SetPingHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(PongWait))
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(WriteWait))
	})
	conn.SetReadDeadline(time.Now().Add(PongWait))

	regBody, _ := json.Marshal(runnerproto.RegisterRequest{
		UserID:      userID,
		AuthToken:   authToken,
		Workspace:   workspace,
		Shell:       shell,
		LLMProvider: opt.LLMProvider,
		LLMModel:    opt.LLMModel,
	})
	regMsg, _ := json.Marshal(runnerproto.RunnerMessage{
		Type:   "register",
		UserID: userID,
		Body:   regBody,
	})
	if err := conn.WriteMessage(websocket.TextMessage, regMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send registration: %w", err)
	}

	// Wait for server confirmation or rejection
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	_, raw, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("waiting for registration response: %w", err)
	}
	var resp runnerproto.RunnerMessage
	if err := json.Unmarshal(raw, &resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid registration response: %w", err)
	}
	if resp.Type == "error" {
		var e runnerproto.ErrorResponse
		json.Unmarshal(resp.Body, &e)
		conn.Close()
		return nil, fmt.Errorf("registration rejected: %s", e.Message)
	}

	// Reset read timeout to normal-operation pongWait
	conn.SetReadDeadline(time.Now().Add(PongWait))

	callLogf(opt.LogFunc, "Registration sent  user=%s  workspace=%s", userID, workspace)
	return conn, nil
}

// WritePump is the only goroutine that writes to the WebSocket connection.
// All writes (responses, heartbeats) go through writeCh to avoid concurrent writes.
func WritePump(conn *websocket.Conn, writeCh <-chan WriteMsg, stop <-chan struct{}, done chan<- struct{}, logf LogFunc) {
	ticker := time.NewTicker(PingPeriod)
	defer func() {
		ticker.Stop()
		conn.Close()
		close(done)
	}()

	for {
		select {
		case msg := <-writeCh:
			if msg.Err != nil {
				// Control message (ping) — use WriteControl
				err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(WriteWait))
				msg.Err <- err
			} else {
				err := conn.WriteMessage(websocket.TextMessage, msg.Data)
				if err != nil {
					callLogf(logf, "WebSocket write error: %v", err)
					return
				}
			}
		case <-ticker.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(WriteWait)); err != nil {
				callLogf(logf, "Ping failed: %v", err)
				return
			}
		case <-stop:
			return
		}
	}
}

// ReadLoop reads messages from the server and dispatches to handler.
// Requests are processed asynchronously so the read loop can continue handling WebSocket control frames (ping/pong) during long operations.
func ReadLoop(conn *websocket.Conn, handler *Handler, writeCh chan<- WriteMsg, writeDone <-chan struct{}, logf LogFunc) {
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				callLogf(logf, "WebSocket read error: %v", err)
			} else {
				callLogf(logf, "WebSocket closed: %v", err)
			}
			return
		}

		var msg runnerproto.RunnerMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			callLogf(logf, "Invalid message from server: %v", err)
			continue
		}

		if handler.Verbose {
			callLogf(logf, "→ %s [id=%s]", msg.Type, msg.ID)
		}

		// Fire-and-forget messages (no response needed)
		if msg.Type == runnerproto.ProtoStdioWrite {
			go handler.DispatchFireAndForget(msg)
			continue
		}

		go func() {
			resp := handler.HandleRequest(msg)
			data, _ := json.Marshal(resp)
			select {
			case writeCh <- WriteMsg{Data: data}:
			case <-writeDone:
			}
		}()
	}
}
