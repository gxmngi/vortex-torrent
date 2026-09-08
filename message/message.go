package message

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MessageID represents standard BitTorrent wire protocol message codes.
type MessageID uint8

const (
	MsgChoke         MessageID = 0
	MsgUnchoke       MessageID = 1
	MsgInterested    MessageID = 2
	MsgNotInterested MessageID = 3
	MsgHave          MessageID = 4
	MsgBitfield      MessageID = 5
	MsgRequest       MessageID = 6
	MsgPiece         MessageID = 7
	MsgCancel        MessageID = 8
)

// Message represents a BitTorrent peer wire message.
// Format: <length prefix: 4 bytes><message ID: 1 byte><payload>
type Message struct {
	ID      MessageID
	Payload []byte
}

// FormatRequest creates a Request message asking the peer for a 16KB block.
// Payload format: [4B piece index][4B byte offset begin][4B block length]
func FormatRequest(index, begin, length int) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return &Message{ID: MsgRequest, Payload: payload}
}

// FormatCancel creates a Cancel message telling a peer to drop a requested block (BEP 0003).
// Payload format: [4B piece index][4B byte offset begin][4B block length]
func FormatCancel(index, begin, length int) *Message {
	payload := make([]byte, 12)
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	binary.BigEndian.PutUint32(payload[8:12], uint32(length))
	return &Message{ID: MsgCancel, Payload: payload}
}

// ParseCancel extracts the piece index, begin offset, and block length from a Cancel message.
func ParseCancel(msg *Message) (int, int, int, error) {
	if msg.ID != MsgCancel {
		return 0, 0, 0, fmt.Errorf("expected Cancel (ID %d), got ID %d", MsgCancel, msg.ID)
	}
	if len(msg.Payload) != 12 {
		return 0, 0, 0, fmt.Errorf("expected payload length 12, got %d", len(msg.Payload))
	}
	index := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	length := int(binary.BigEndian.Uint32(msg.Payload[8:12]))
	return index, begin, length, nil
}

// FormatHave creates a Have message informing the peer that we have downloaded a piece.
func FormatHave(index int) *Message {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(index))
	return &Message{ID: MsgHave, Payload: payload}
}

// ParseHave extracts the piece index from a Have message payload.
func ParseHave(msg *Message) (int, error) {
	if msg.ID != MsgHave {
		return 0, fmt.Errorf("expected Have (ID %d), got ID %d", MsgHave, msg.ID)
	}
	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("expected payload length 4, got %d", len(msg.Payload))
	}
	return int(binary.BigEndian.Uint32(msg.Payload)), nil
}

// ParsePiece copies the block payload from a Piece message into a destination buffer.
// Payload format: [4B piece index][4B byte offset begin][block data...]
func ParsePiece(index int, buf []byte, msg *Message) (int, error) {
	if msg.ID != MsgPiece {
		return 0, fmt.Errorf("expected Piece (ID %d), got ID %d", MsgPiece, msg.ID)
	}
	if len(msg.Payload) < 8 {
		return 0, fmt.Errorf("piece message payload too short: %d bytes", len(msg.Payload))
	}

	parsedIndex := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
	if parsedIndex != index {
		return 0, fmt.Errorf("expected piece index %d, got %d", index, parsedIndex)
	}

	begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
	if begin >= len(buf) {
		return 0, fmt.Errorf("begin offset %d exceeds buffer size %d", begin, len(buf))
	}

	data := msg.Payload[8:]
	if begin+len(data) > len(buf) {
		return 0, fmt.Errorf("data too long for buffer: offset %d + len %d > %d", begin, len(data), len(buf))
	}

	copy(buf[begin:], data)
	return len(data), nil
}

// ParsePieceBegin extracts the byte offset begin from a Piece message payload.
func ParsePieceBegin(msg *Message) int {
	if msg == nil || len(msg.Payload) < 8 {
		return 0
	}
	return int(binary.BigEndian.Uint32(msg.Payload[4:8]))
}

// Serialize encodes a message into wire protocol bytes.
// If msg is nil, it encodes a KeepAlive message (4 zero bytes).
func (m *Message) Serialize() []byte {
	if m == nil {
		return make([]byte, 4)
	}

	length := uint32(len(m.Payload) + 1)
	buf := make([]byte, 4+length)
	binary.BigEndian.PutUint32(buf[0:4], length)
	buf[4] = byte(m.ID)
	copy(buf[5:], m.Payload)
	return buf
}

// Read reads a wire protocol message from an incoming stream.
func Read(r io.Reader) (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(lengthBuf)
	// Keep-alive message has length 0
	if length == 0 {
		return nil, nil
	}

	messageBuf := make([]byte, length)
	_, err = io.ReadFull(r, messageBuf)
	if err != nil {
		return nil, err
	}

	return &Message{
		ID:      MessageID(messageBuf[0]),
		Payload: messageBuf[1:],
	}, nil
}
