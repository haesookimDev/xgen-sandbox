package protocol

import (
	"encoding/binary"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

const headerSize = 9 // 1 (type) + 4 (channel) + 4 (id)

// Encode serializes an Envelope into binary wire format.
func Encode(e *Envelope) ([]byte, error) {
	buf := make([]byte, headerSize+len(e.Payload))
	buf[0] = e.Type
	binary.BigEndian.PutUint32(buf[1:5], e.Channel)
	binary.BigEndian.PutUint32(buf[5:9], e.ID)
	copy(buf[headerSize:], e.Payload)
	return buf, nil
}

// Decode deserializes binary wire format into an Envelope.
func Decode(data []byte) (*Envelope, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("message too short: %d bytes", len(data))
	}
	return &Envelope{
		Type:    data[0],
		Channel: binary.BigEndian.Uint32(data[1:5]),
		ID:      binary.BigEndian.Uint32(data[5:9]),
		Payload: data[headerSize:],
	}, nil
}

// EncodePayload marshals a payload struct into msgpack bytes.
func EncodePayload(v any) ([]byte, error) {
	return msgpack.Marshal(v)
}

// DecodePayload unmarshals msgpack bytes into a payload struct.
func DecodePayload(data []byte, v any) error {
	return msgpack.Unmarshal(data, v)
}

// NewEnvelope creates a new envelope with an encoded payload.
func NewEnvelope(msgType uint8, channel, id uint32, payload any) (*Envelope, error) {
	var raw []byte
	if payload != nil {
		var err error
		raw, err = EncodePayload(payload)
		if err != nil {
			return nil, fmt.Errorf("encode payload: %w", err)
		}
	}
	return &Envelope{
		Type:    msgType,
		Channel: channel,
		ID:      id,
		Payload: raw,
	}, nil
}
