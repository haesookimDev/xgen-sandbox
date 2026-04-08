package protocol

import (
	"encoding/binary"
	"testing"
)

func TestEncode_ValidEnvelope(t *testing.T) {
	e := &Envelope{
		Type:    MsgExecStart,
		Channel: 42,
		ID:      7,
		Payload: []byte("hello"),
	}

	data, err := Encode(e)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	if len(data) != headerSize+5 {
		t.Fatalf("expected length %d, got %d", headerSize+5, len(data))
	}
	if data[0] != MsgExecStart {
		t.Errorf("type: expected 0x%02x, got 0x%02x", MsgExecStart, data[0])
	}
	if ch := binary.BigEndian.Uint32(data[1:5]); ch != 42 {
		t.Errorf("channel: expected 42, got %d", ch)
	}
	if id := binary.BigEndian.Uint32(data[5:9]); id != 7 {
		t.Errorf("id: expected 7, got %d", id)
	}
	if string(data[headerSize:]) != "hello" {
		t.Errorf("payload: expected %q, got %q", "hello", string(data[headerSize:]))
	}
}

func TestDecode_ValidMessage(t *testing.T) {
	data := make([]byte, headerSize+3)
	data[0] = MsgExecStdout
	binary.BigEndian.PutUint32(data[1:5], 100)
	binary.BigEndian.PutUint32(data[5:9], 200)
	copy(data[headerSize:], "abc")

	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if env.Type != MsgExecStdout {
		t.Errorf("type: expected 0x%02x, got 0x%02x", MsgExecStdout, env.Type)
	}
	if env.Channel != 100 {
		t.Errorf("channel: expected 100, got %d", env.Channel)
	}
	if env.ID != 200 {
		t.Errorf("id: expected 200, got %d", env.ID)
	}
	if string(env.Payload) != "abc" {
		t.Errorf("payload: expected %q, got %q", "abc", string(env.Payload))
	}
}

func TestDecode_TooShort(t *testing.T) {
	_, err := Decode([]byte{0x01, 0x02})
	if err == nil {
		t.Fatal("expected error for short message, got nil")
	}
}

func TestEncodeDecode_Roundtrip(t *testing.T) {
	types := []uint8{MsgPing, MsgExecStart, MsgFsRead, MsgPortOpen, MsgSandboxReady}

	for _, msgType := range types {
		e := &Envelope{
			Type:    msgType,
			Channel: 12345,
			ID:      67890,
			Payload: []byte("test-payload"),
		}

		data, err := Encode(e)
		if err != nil {
			t.Fatalf("Encode(type=0x%02x) error: %v", msgType, err)
		}

		decoded, err := Decode(data)
		if err != nil {
			t.Fatalf("Decode(type=0x%02x) error: %v", msgType, err)
		}

		if decoded.Type != e.Type {
			t.Errorf("type mismatch: expected 0x%02x, got 0x%02x", e.Type, decoded.Type)
		}
		if decoded.Channel != e.Channel {
			t.Errorf("channel mismatch: expected %d, got %d", e.Channel, decoded.Channel)
		}
		if decoded.ID != e.ID {
			t.Errorf("id mismatch: expected %d, got %d", e.ID, decoded.ID)
		}
		if string(decoded.Payload) != string(e.Payload) {
			t.Errorf("payload mismatch: expected %q, got %q", e.Payload, decoded.Payload)
		}
	}
}

func TestEncodeDecodePayload_Roundtrip(t *testing.T) {
	original := ExecStartPayload{
		Command: "node",
		Args:    []string{"index.js"},
		Env:     map[string]string{"NODE_ENV": "production"},
		Cwd:     "/app",
		TTY:     true,
		Cols:    120,
		Rows:    40,
	}

	data, err := EncodePayload(original)
	if err != nil {
		t.Fatalf("EncodePayload() error: %v", err)
	}

	var decoded ExecStartPayload
	if err := DecodePayload(data, &decoded); err != nil {
		t.Fatalf("DecodePayload() error: %v", err)
	}

	if decoded.Command != original.Command {
		t.Errorf("command: expected %q, got %q", original.Command, decoded.Command)
	}
	if len(decoded.Args) != 1 || decoded.Args[0] != "index.js" {
		t.Errorf("args: expected %v, got %v", original.Args, decoded.Args)
	}
	if decoded.Env["NODE_ENV"] != "production" {
		t.Errorf("env: expected production, got %v", decoded.Env)
	}
	if decoded.Cwd != "/app" {
		t.Errorf("cwd: expected /app, got %q", decoded.Cwd)
	}
	if !decoded.TTY {
		t.Error("tty: expected true")
	}
	if decoded.Cols != 120 || decoded.Rows != 40 {
		t.Errorf("size: expected 120x40, got %dx%d", decoded.Cols, decoded.Rows)
	}
}

func TestNewEnvelope_NilPayload(t *testing.T) {
	env, err := NewEnvelope(MsgPing, 0, 0, nil)
	if err != nil {
		t.Fatalf("NewEnvelope() error: %v", err)
	}
	if len(env.Payload) != 0 {
		t.Errorf("expected empty payload, got %d bytes", len(env.Payload))
	}
}

func TestNewEnvelope_WithPayload(t *testing.T) {
	payload := ExecExitPayload{ExitCode: 42}
	env, err := NewEnvelope(MsgExecExit, 1, 2, payload)
	if err != nil {
		t.Fatalf("NewEnvelope() error: %v", err)
	}

	var decoded ExecExitPayload
	if err := DecodePayload(env.Payload, &decoded); err != nil {
		t.Fatalf("DecodePayload() error: %v", err)
	}
	if decoded.ExitCode != 42 {
		t.Errorf("exit code: expected 42, got %d", decoded.ExitCode)
	}
}

func TestDecode_EmptyPayload(t *testing.T) {
	data := make([]byte, headerSize)
	data[0] = MsgPong

	env, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}
	if len(env.Payload) != 0 {
		t.Errorf("expected empty payload, got %d bytes", len(env.Payload))
	}
}
