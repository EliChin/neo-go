package stateroot

import (
	"fmt"

	"github.com/nspcc-dev/neo-go/pkg/core/state"
	"github.com/nspcc-dev/neo-go/pkg/io"
)

type (
	// MessageType represents message type.
	MessageType byte

	// Message represents state-root related message.
	Message struct {
		Type    MessageType
		Payload io.Serializable
	}
)

// Various message types.
const (
	VoteT MessageType = 0
	RootT MessageType = 1
)

// NewMessage creates new message of specified type.
func NewMessage(typ MessageType, p io.Serializable) *Message {
	return &Message{
		Type:    typ,
		Payload: p,
	}
}

// EncodeBinary implements io.Serializable interface.
func (m *Message) EncodeBinary(w io.BinaryWriter) {
	w.WriteB(byte(m.Type))
	m.Payload.EncodeBinary(w)
}

// DecodeBinary implements io.Serializable interface.
func (m *Message) DecodeBinary(r io.BinaryReader) {
	switch m.Type = MessageType(r.ReadB()); m.Type {
	case VoteT:
		m.Payload = new(Vote)
	case RootT:
		m.Payload = new(state.MPTRoot)
	default:
		r.SetError(fmt.Errorf("invalid type: %x", m.Type))
		return
	}
	m.Payload.DecodeBinary(r)
}
