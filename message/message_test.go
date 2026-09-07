package message

import (
	"bytes"
	"testing"
)

func TestHandshakeSerializeAndRead(t *testing.T) {
	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	peerID := [20]byte{'V', 'T', '-', '0', '0', '0', '1', '-', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'a', 'b'}

	hs := NewHandshake(infoHash, peerID)
	serialized := hs.Serialize()

	if len(serialized) != HandshakeLength {
		t.Fatalf("expected length %d, got %d", HandshakeLength, len(serialized))
	}

	reader := bytes.NewReader(serialized)
	parsed, err := ReadHandshake(reader)
	if err != nil {
		t.Fatalf("ReadHandshake failed: %v", err)
	}

	if parsed.Pstr != ProtocolString {
		t.Errorf("expected protocol string %q, got %q", ProtocolString, parsed.Pstr)
	}
	if parsed.InfoHash != infoHash {
		t.Errorf("info hash mismatch")
	}
	if parsed.PeerID != peerID {
		t.Errorf("peer id mismatch")
	}
}

func TestFormatAndParseRequest(t *testing.T) {
	msg := FormatRequest(4, 0x4000, 16384)
	serialized := msg.Serialize()

	reader := bytes.NewReader(serialized)
	readMsg, err := Read(reader)
	if err != nil {
		t.Fatalf("Read message failed: %v", err)
	}

	if readMsg.ID != MsgRequest {
		t.Errorf("expected ID %d, got %d", MsgRequest, readMsg.ID)
	}
}

func TestParsePiece(t *testing.T) {
	// Construct a mock piece block
	blockData := []byte("Hello BitTorrent Block!")
	msg := &Message{
		ID: MsgPiece,
		Payload: append(
			[]byte{0, 0, 0, 1, 0, 0, 0, 10}, // index=1, begin=10
			blockData...,
		),
	}

	destBuf := make([]byte, 50)
	n, err := ParsePiece(1, destBuf, msg)
	if err != nil {
		t.Fatalf("ParsePiece error: %v", err)
	}

	if n != len(blockData) {
		t.Errorf("expected %d bytes copied, got %d", len(blockData), n)
	}

	if string(destBuf[10:10+len(blockData)]) != string(blockData) {
		t.Errorf("buffer mismatch: got %q", string(destBuf[10:10+len(blockData)]))
	}
}
