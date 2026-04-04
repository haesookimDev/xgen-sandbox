package transport

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/xgen-sandbox/sdk-go/protocol"
	"nhooyr.io/websocket"
)

// MessageHandler is a callback for incoming WebSocket messages.
type MessageHandler func(env protocol.Envelope)

// WSTransport manages a WebSocket connection with the binary protocol.
type WSTransport struct {
	url       string
	token     string
	conn      *websocket.Conn
	mu        sync.Mutex // protects writes
	nextID    atomic.Uint32
	handlers  sync.Map // byte -> *handlerList
	pending   sync.Map // uint32 -> *pendingRequest
	done      chan struct{}
	closeOnce sync.Once
}

type handlerList struct {
	mu       sync.Mutex
	handlers []MessageHandler
}

type pendingRequest struct {
	ch chan protocol.Envelope
}

// NewWSTransport creates a new WebSocket transport.
func NewWSTransport(wsURL, token string) *WSTransport {
	ws := &WSTransport{
		url:   wsURL,
		token: token,
		done:  make(chan struct{}),
	}
	ws.nextID.Store(1)
	return ws
}

// Connect establishes the WebSocket connection and starts the read loop.
func (ws *WSTransport) Connect(ctx context.Context) error {
	u, err := url.Parse(ws.url)
	if err != nil {
		return fmt.Errorf("invalid ws url: %w", err)
	}
	q := u.Query()
	q.Set("token", ws.token)
	u.RawQuery = q.Encode()

	conn, _, err := websocket.Dial(ctx, u.String(), &websocket.DialOptions{})
	if err != nil {
		return fmt.Errorf("ws connect failed: %w", err)
	}
	conn.SetReadLimit(16 * 1024 * 1024) // 16MB
	ws.conn = conn

	go ws.readLoop()
	return nil
}

// Close closes the WebSocket connection.
func (ws *WSTransport) Close() error {
	var err error
	ws.closeOnce.Do(func() {
		close(ws.done)
		if ws.conn != nil {
			err = ws.conn.Close(websocket.StatusNormalClosure, "closing")
		}
		// Reject all pending requests.
		ws.pending.Range(func(key, value any) bool {
			ws.pending.Delete(key)
			return true
		})
	})
	return err
}

// Send writes an envelope to the WebSocket connection.
func (ws *WSTransport) Send(ctx context.Context, env protocol.Envelope) error {
	data := protocol.EncodeEnvelope(env)
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.conn.Write(ctx, websocket.MessageBinary, data)
}

// Request sends an envelope and waits for a matching response by ID.
func (ws *WSTransport) Request(ctx context.Context, msgType byte, channel uint32, payload []byte) (protocol.Envelope, error) {
	id := ws.nextID.Add(1) - 1

	pr := &pendingRequest{ch: make(chan protocol.Envelope, 1)}
	ws.pending.Store(id, pr)
	defer ws.pending.Delete(id)

	err := ws.Send(ctx, protocol.Envelope{
		Type:    msgType,
		Channel: channel,
		ID:      id,
		Payload: payload,
	})
	if err != nil {
		return protocol.Envelope{}, err
	}

	select {
	case env := <-pr.ch:
		if env.Type == protocol.Error {
			return env, fmt.Errorf("server error (id=%d)", id)
		}
		return env, nil
	case <-ctx.Done():
		return protocol.Envelope{}, ctx.Err()
	case <-ws.done:
		return protocol.Envelope{}, fmt.Errorf("connection closed")
	}
}

// On registers a handler for a specific message type. Returns a function to unregister.
func (ws *WSTransport) On(msgType byte, handler MessageHandler) func() {
	val, _ := ws.handlers.LoadOrStore(msgType, &handlerList{})
	hl := val.(*handlerList)

	hl.mu.Lock()
	hl.handlers = append(hl.handlers, handler)
	hl.mu.Unlock()

	return func() {
		hl.mu.Lock()
		defer hl.mu.Unlock()
		for i, h := range hl.handlers {
			// Compare function pointers via fmt sprint (Go doesn't allow direct comparison).
			if fmt.Sprintf("%p", h) == fmt.Sprintf("%p", handler) {
				hl.handlers = append(hl.handlers[:i], hl.handlers[i+1:]...)
				break
			}
		}
	}
}

func (ws *WSTransport) readLoop() {
	for {
		select {
		case <-ws.done:
			return
		default:
		}

		_, data, err := ws.conn.Read(context.Background())
		if err != nil {
			// Connection closed or error; stop the loop.
			ws.closeOnce.Do(func() { close(ws.done) })
			return
		}

		env, err := protocol.DecodeEnvelope(data)
		if err != nil {
			continue
		}

		ws.handleMessage(env)
	}
}

func (ws *WSTransport) handleMessage(env protocol.Envelope) {
	// Handle ping with pong.
	if env.Type == protocol.Ping {
		_ = ws.Send(context.Background(), protocol.Envelope{
			Type:    protocol.Pong,
			Channel: 0,
			ID:      env.ID,
			Payload: nil,
		})
		return
	}

	// Check for pending request match.
	if env.ID > 0 {
		if val, ok := ws.pending.LoadAndDelete(env.ID); ok {
			pr := val.(*pendingRequest)
			pr.ch <- env
			return
		}
	}

	// Dispatch to type handlers.
	if val, ok := ws.handlers.Load(env.Type); ok {
		hl := val.(*handlerList)
		hl.mu.Lock()
		handlers := make([]MessageHandler, len(hl.handlers))
		copy(handlers, hl.handlers)
		hl.mu.Unlock()

		for _, h := range handlers {
			h(env)
		}
	}
}
