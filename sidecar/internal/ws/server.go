package ws

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/vmihailenco/msgpack/v5"
	"nhooyr.io/websocket"

	execpkg "github.com/xgen-sandbox/sidecar/internal/exec"
	fspkg "github.com/xgen-sandbox/sidecar/internal/fs"
	"github.com/xgen-sandbox/sidecar/internal/port"
)

// Message types (mirrored from agent/pkg/protocol)
const (
	MsgPing         uint8 = 0x01
	MsgPong         uint8 = 0x02
	MsgError        uint8 = 0x03
	MsgAck          uint8 = 0x04
	MsgExecStart    uint8 = 0x20
	MsgExecStdin    uint8 = 0x21
	MsgExecStdout   uint8 = 0x22
	MsgExecStderr   uint8 = 0x23
	MsgExecExit     uint8 = 0x24
	MsgExecSignal   uint8 = 0x25
	MsgExecResize   uint8 = 0x26
	MsgFsRead       uint8 = 0x30
	MsgFsWrite      uint8 = 0x31
	MsgFsList       uint8 = 0x32
	MsgFsRemove     uint8 = 0x33
	MsgFsWatch      uint8 = 0x34
	MsgFsEvent      uint8 = 0x35
	MsgPortOpen     uint8 = 0x40
	MsgPortClose    uint8 = 0x41
	MsgSandboxReady uint8 = 0x50
	MsgSandboxError uint8 = 0x51
)

const headerSize = 9

type envelope struct {
	Type    uint8
	Channel uint32
	ID      uint32
	Payload []byte
	conn    *websocket.Conn // originating connection for replies
	state   *connState      // originating connection state for replies/events
}

func decodeEnvelope(data []byte) (*envelope, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("message too short: %d", len(data))
	}
	return &envelope{
		Type:    data[0],
		Channel: binary.BigEndian.Uint32(data[1:5]),
		ID:      binary.BigEndian.Uint32(data[5:9]),
		Payload: data[headerSize:],
	}, nil
}

func encodeEnvelope(e *envelope) []byte {
	buf := make([]byte, headerSize+len(e.Payload))
	buf[0] = e.Type
	binary.BigEndian.PutUint32(buf[1:5], e.Channel)
	binary.BigEndian.PutUint32(buf[5:9], e.ID)
	copy(buf[headerSize:], e.Payload)
	return buf
}

// connState holds per-connection state that is isolated from other connections.
type connState struct {
	conn    *websocket.Conn
	portDet *port.Detector
	watcher *fspkg.Watcher
	writeMu sync.Mutex
}

// Server is the sidecar WebSocket server that handles agent connections.
type Server struct {
	execMgr          *execpkg.Manager
	fsHandler        *fspkg.Handler
	chanID           atomic.Uint32
	channelMu        sync.RWMutex
	channelProcesses map[uint32]uint32
}

// NewServer creates a new sidecar WebSocket server.
func NewServer(execMgr *execpkg.Manager, fsHandler *fspkg.Handler) *Server {
	s := &Server{
		execMgr:          execMgr,
		fsHandler:        fsHandler,
		channelProcesses: make(map[uint32]uint32),
	}
	s.chanID.Store(100)
	return s
}

// Handler returns an http.Handler for the WebSocket endpoint.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // Agent is trusted; auth is at the Agent layer
		})
		if err != nil {
			log.Printf("websocket accept: %v", err)
			return
		}
		defer conn.CloseNow()

		log.Printf("ws connection accepted from %s", r.RemoteAddr)

		cs := &connState{conn: conn}

		// Start port detector
		cs.portDet = port.NewDetector(
			func(p uint16) { s.sendPortEventTo(cs, MsgPortOpen, p) },
			func(p uint16) { s.sendPortEventTo(cs, MsgPortClose, p) },
		)
		cs.portDet.Start()
		defer cs.portDet.Stop()

		// Start file watcher
		cs.watcher = fspkg.NewWatcher("/home/sandbox/workspace", func(evt fspkg.FsEvent) {
			payload, _ := msgpack.Marshal(evt)
			s.sendEnvelope(&envelope{Type: MsgFsEvent, Payload: payload, conn: conn, state: cs})
		})
		cs.watcher.Start()
		defer cs.watcher.Stop()

		// Send ready signal
		s.sendEnvelope(&envelope{Type: MsgSandboxReady, conn: conn, state: cs})

		ctx := r.Context()
		s.readLoop(ctx, cs)
	})
}

func (s *Server) readLoop(ctx context.Context, cs *connState) {
	conn := cs.conn
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) != -1 {
				log.Printf("websocket closed: %v", err)
			}
			return
		}

		env, err := decodeEnvelope(data)
		if err != nil {
			log.Printf("decode error: %v", err)
			continue
		}

		env.conn = conn
		env.state = cs
		log.Printf("recv msg type=0x%02x channel=%d id=%d payloadLen=%d", env.Type, env.Channel, env.ID, len(env.Payload))
		go s.handleMessage(ctx, env)
	}
}

func (s *Server) handleMessage(ctx context.Context, env *envelope) {
	switch env.Type {
	case MsgPing:
		s.sendEnvelope(&envelope{Type: MsgPong, Channel: env.Channel, ID: env.ID, conn: env.conn, state: env.state})

	case MsgExecStart:
		s.handleExecStart(ctx, env)

	case MsgExecStdin:
		s.handleExecStdin(env)

	case MsgExecSignal:
		s.handleExecSignal(env)

	case MsgExecResize:
		s.handleExecResize(env)

	case MsgFsRead:
		s.handleFsRead(env)

	case MsgFsWrite:
		s.handleFsWrite(env)

	case MsgFsList:
		s.handleFsList(env)

	case MsgFsRemove:
		s.handleFsRemove(env)

	case MsgFsWatch:
		s.handleFsWatch(env)

	default:
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "unknown_message", fmt.Sprintf("unknown message type: 0x%02x", env.Type))
	}
}

// --- Exec handlers ---

type execStartPayload struct {
	Command string            `msgpack:"command"`
	Args    []string          `msgpack:"args"`
	Env     map[string]string `msgpack:"env,omitempty"`
	Cwd     string            `msgpack:"cwd,omitempty"`
	TTY     bool              `msgpack:"tty"`
	Cols    uint16            `msgpack:"cols,omitempty"`
	Rows    uint16            `msgpack:"rows,omitempty"`
}

func (s *Server) handleExecStart(ctx context.Context, env *envelope) {
	var payload execStartPayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}

	proc, err := s.execMgr.Start(execpkg.StartOptions{
		Command: payload.Command,
		Args:    payload.Args,
		Env:     payload.Env,
		Cwd:     payload.Cwd,
		TTY:     payload.TTY,
		Cols:    payload.Cols,
		Rows:    payload.Rows,
	})
	if err != nil {
		log.Printf("exec start failed: %v (command=%s args=%v)", err, payload.Command, payload.Args)
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "exec_failed", err.Error())
		return
	}

	// Use the channel ID from the envelope for this exec session
	chanID := env.Channel
	if chanID == 0 {
		chanID = s.chanID.Add(1)
	}
	s.registerChannelProcess(chanID, proc.ID)

	// Capture the originating connection for all responses in this exec session
	replyConn := env.conn
	replyState := env.state

	// ACK with the channel and process ID
	ackPayload, _ := msgpack.Marshal(map[string]uint32{"process_id": proc.ID, "channel": chanID})
	s.sendEnvelope(&envelope{Type: MsgAck, Channel: chanID, ID: env.ID, Payload: ackPayload, conn: replyConn, state: replyState})

	// Stream stdout/stderr, tracking completion with a WaitGroup so that
	// ExecExit is only sent after all output has been flushed to the client.
	var wg sync.WaitGroup

	if stdout := proc.Stdout(); stdout != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.streamOutputTo(ctx, replyConn, replyState, stdout, MsgExecStdout, chanID, proc)
		}()
	}

	if stderr := proc.Stderr(); stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.streamOutputTo(ctx, replyConn, replyState, stderr, MsgExecStderr, chanID, proc)
		}()
	}

	// Wait for exit, then wait for streams to finish before sending ExecExit
	go func() {
		<-proc.Done()
		wg.Wait()
		exitPayload, _ := msgpack.Marshal(map[string]int{"exit_code": proc.ExitCode()})
		s.sendEnvelope(&envelope{Type: MsgExecExit, Channel: chanID, Payload: exitPayload, conn: replyConn, state: replyState})
		s.unregisterChannelProcess(chanID)
		proc.Close()
		s.execMgr.Remove(proc.ID)
	}()
}

func (s *Server) streamOutputTo(ctx context.Context, conn *websocket.Conn, state *connState, r io.Reader, msgType uint8, chanID uint32, proc *execpkg.Process) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			s.sendEnvelope(&envelope{Type: msgType, Channel: chanID, Payload: data, conn: conn, state: state})
		}
		if err != nil {
			return
		}
	}
}

func (s *Server) handleExecStdin(env *envelope) {
	if len(env.Payload) == 0 {
		return
	}
	var procID uint32
	input := env.Payload
	if len(env.Payload) >= 4 {
		procID = binary.BigEndian.Uint32(env.Payload[:4])
		input = env.Payload[4:]
	}
	proc, ok := s.processFor(procID, env.Channel)
	if !ok {
		return
	}
	proc.WriteStdin(input)
}

func (s *Server) processFor(procID, channel uint32) (*execpkg.Process, bool) {
	resolved, ok := s.resolveProcessID(procID, channel)
	if !ok {
		return nil, false
	}
	return s.execMgr.Get(resolved)
}

func (s *Server) resolveProcessID(procID, channel uint32) (uint32, bool) {
	if procID != 0 {
		return procID, true
	}
	if channel == 0 {
		return 0, false
	}
	s.channelMu.RLock()
	defer s.channelMu.RUnlock()
	resolved, ok := s.channelProcesses[channel]
	return resolved, ok
}

func (s *Server) registerChannelProcess(channel, procID uint32) {
	if channel == 0 || procID == 0 {
		return
	}
	s.channelMu.Lock()
	s.channelProcesses[channel] = procID
	s.channelMu.Unlock()
}

func (s *Server) unregisterChannelProcess(channel uint32) {
	if channel == 0 {
		return
	}
	s.channelMu.Lock()
	delete(s.channelProcesses, channel)
	s.channelMu.Unlock()
}

func (s *Server) handleExecSignal(env *envelope) {
	var payload struct {
		ProcessID uint32 `msgpack:"process_id"`
		Signal    string `msgpack:"signal"`
	}
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	proc, ok := s.processFor(payload.ProcessID, env.Channel)
	if !ok {
		return
	}
	switch payload.Signal {
	case "SIGTERM":
		proc.Signal(15)
	case "SIGKILL":
		proc.Kill()
	case "SIGINT":
		proc.Signal(2)
	}
}

func (s *Server) handleExecResize(env *envelope) {
	var payload struct {
		ProcessID uint32 `msgpack:"process_id"`
		Cols      uint16 `msgpack:"cols"`
		Rows      uint16 `msgpack:"rows"`
	}
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		return
	}
	proc, ok := s.processFor(payload.ProcessID, env.Channel)
	if !ok {
		return
	}
	proc.Resize(payload.Cols, payload.Rows)
}

// --- Filesystem handlers ---

type fsReadPayload struct {
	Path string `msgpack:"path"`
}

func (s *Server) handleFsRead(env *envelope) {
	var payload fsReadPayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}

	content, err := s.fsHandler.ReadFile(payload.Path)
	if err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "fs_error", err.Error())
		return
	}

	s.sendEnvelope(&envelope{Type: MsgFsRead, Channel: env.Channel, ID: env.ID, Payload: content, conn: env.conn, state: env.state})
}

type fsWritePayload struct {
	Path    string `msgpack:"path"`
	Content []byte `msgpack:"content"`
	Mode    uint32 `msgpack:"mode,omitempty"`
}

func (s *Server) handleFsWrite(env *envelope) {
	var payload fsWritePayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}

	if err := s.fsHandler.WriteFile(payload.Path, payload.Content, 0644); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "fs_error", err.Error())
		return
	}

	s.sendEnvelope(&envelope{Type: MsgAck, Channel: env.Channel, ID: env.ID, conn: env.conn, state: env.state})
}

type fsListPayload struct {
	Path string `msgpack:"path"`
}

func (s *Server) handleFsList(env *envelope) {
	var payload fsListPayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}

	entries, err := s.fsHandler.ListDir(payload.Path)
	if err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "fs_error", err.Error())
		return
	}

	data, _ := msgpack.Marshal(entries)
	s.sendEnvelope(&envelope{Type: MsgFsList, Channel: env.Channel, ID: env.ID, Payload: data, conn: env.conn, state: env.state})
}

type fsRemovePayload struct {
	Path      string `msgpack:"path"`
	Recursive bool   `msgpack:"recursive"`
}

func (s *Server) handleFsRemove(env *envelope) {
	var payload fsRemovePayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}

	if err := s.fsHandler.Remove(payload.Path, payload.Recursive); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "fs_error", err.Error())
		return
	}

	s.sendEnvelope(&envelope{Type: MsgAck, Channel: env.Channel, ID: env.ID, conn: env.conn, state: env.state})
}

// --- File watch handler ---

type fsWatchPayload struct {
	Path    string `msgpack:"path"`
	Unwatch bool   `msgpack:"unwatch,omitempty"`
}

func (s *Server) handleFsWatch(env *envelope) {
	var payload fsWatchPayload
	if err := msgpack.Unmarshal(env.Payload, &payload); err != nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "invalid_payload", err.Error())
		return
	}
	var watcher *fspkg.Watcher
	if env.state != nil {
		watcher = env.state.watcher
	}
	if watcher == nil {
		s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "watcher_error", "watcher not initialized")
		return
	}
	if payload.Unwatch {
		watcher.Unwatch(payload.Path)
	} else {
		if err := watcher.Watch(payload.Path); err != nil {
			s.sendErrorTo(env.conn, env.state, env.Channel, env.ID, "watch_error", err.Error())
			return
		}
	}
	s.sendEnvelope(&envelope{Type: MsgAck, Channel: env.Channel, ID: env.ID, conn: env.conn, state: env.state})
}

// --- Helpers ---

func (s *Server) sendEnvelope(env *envelope) {
	conn := env.conn
	if conn == nil && env.state != nil {
		conn = env.state.conn
	}

	if conn == nil {
		log.Printf("send msg type=0x%02x: no connection", env.Type)
		return
	}

	data := encodeEnvelope(env)
	if env.state != nil {
		env.state.writeMu.Lock()
		defer env.state.writeMu.Unlock()
	}
	if err := conn.Write(context.Background(), websocket.MessageBinary, data); err != nil {
		log.Printf("send msg type=0x%02x: write error: %v", env.Type, err)
	}
}

func (s *Server) sendError(channel, id uint32, code, message string) {
	s.sendErrorTo(nil, nil, channel, id, code, message)
}

func (s *Server) sendErrorTo(conn *websocket.Conn, state *connState, channel, id uint32, code, message string) {
	payload, _ := msgpack.Marshal(map[string]string{"code": code, "message": message})
	s.sendEnvelope(&envelope{Type: MsgError, Channel: channel, ID: id, Payload: payload, conn: conn, state: state})
}

func (s *Server) sendPortEventTo(state *connState, msgType uint8, p uint16) {
	payload, _ := msgpack.Marshal(map[string]uint16{"port": p})
	s.sendEnvelope(&envelope{Type: msgType, Payload: payload, state: state})
}
