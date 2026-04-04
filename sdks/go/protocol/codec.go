package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// HeaderSize is the size of the binary envelope header: type(1) + channel(4) + id(4).
const HeaderSize = 9

// MsgType constants for the binary WebSocket protocol.
const (
	Ping  byte = 0x01
	Pong  byte = 0x02
	Error byte = 0x03
	Ack   byte = 0x04

	ExecStart  byte = 0x20
	ExecStdin  byte = 0x21
	ExecStdout byte = 0x22
	ExecStderr byte = 0x23
	ExecExit   byte = 0x24
	ExecSignal byte = 0x25
	ExecResize byte = 0x26

	FsRead   byte = 0x30
	FsWrite  byte = 0x31
	FsList   byte = 0x32
	FsRemove byte = 0x33
	FsWatch  byte = 0x34
	FsEvent  byte = 0x35

	PortOpen  byte = 0x40
	PortClose byte = 0x41

	SandboxReady byte = 0x50
	SandboxError byte = 0x51
	SandboxStats byte = 0x52
)

// Envelope represents a binary protocol message.
type Envelope struct {
	Type    byte
	Channel uint32
	ID      uint32
	Payload []byte
}

// EncodeEnvelope serializes an Envelope into its binary wire format.
func EncodeEnvelope(env Envelope) []byte {
	buf := make([]byte, HeaderSize+len(env.Payload))
	buf[0] = env.Type
	binary.BigEndian.PutUint32(buf[1:5], env.Channel)
	binary.BigEndian.PutUint32(buf[5:9], env.ID)
	copy(buf[HeaderSize:], env.Payload)
	return buf
}

// DecodeEnvelope deserializes binary data into an Envelope.
func DecodeEnvelope(data []byte) (Envelope, error) {
	if len(data) < HeaderSize {
		return Envelope{}, fmt.Errorf("message too short: %d bytes", len(data))
	}
	return Envelope{
		Type:    data[0],
		Channel: binary.BigEndian.Uint32(data[1:5]),
		ID:      binary.BigEndian.Uint32(data[5:9]),
		Payload: data[HeaderSize:],
	}, nil
}

// EncodePayload serializes a value using msgpack.
func EncodePayload(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

// DecodePayload deserializes msgpack data into the given pointer.
func DecodePayload(data []byte, v any) error {
	if len(data) == 0 {
		return errors.New("empty payload")
	}
	return msgpack.Unmarshal(data, v)
}
