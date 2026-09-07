package message

import (
	"fmt"
	"io"
)

const ProtocolString = "BitTorrent protocol"
const HandshakeLength = 68

// Handshake is the initial 68-byte binary packet exchanged when connecting to a peer.
type Handshake struct {
	Pstr     string
	InfoHash [20]byte
	PeerID   [20]byte
}

// NewHandshake creates a valid BitTorrent handshake structure.
func NewHandshake(infoHash, peerID [20]byte) *Handshake {
	return &Handshake{
		Pstr:     ProtocolString,
		InfoHash: infoHash,
		PeerID:   peerID,
	}
}

// Serialize serializes the handshake into exactly 68 bytes.
// Format: [1B len=19][19B "BitTorrent protocol"][8B reserved=0][20B info_hash][20B peer_id]
func (h *Handshake) Serialize() []byte {
	buf := make([]byte, HandshakeLength)
	buf[0] = byte(len(h.Pstr))
	curr := 1
	curr += copy(buf[curr:], []byte(h.Pstr))
	curr += copy(buf[curr:], make([]byte, 8)) // 8 reserved extension bytes
	curr += copy(buf[curr:], h.InfoHash[:])
	curr += copy(buf[curr:], h.PeerID[:])
	return buf
}

// ReadHandshake reads and parses a 68-byte handshake from the peer's TCP socket.
func ReadHandshake(r io.Reader) (*Handshake, error) {
	lengthBuf := make([]byte, 1)
	_, err := io.ReadFull(r, lengthBuf)
	if err != nil {
		return nil, fmt.Errorf("reading pstrlen: %w", err)
	}

	pstrlen := int(lengthBuf[0])
	if pstrlen != len(ProtocolString) {
		return nil, fmt.Errorf("invalid protocol string length: %d", pstrlen)
	}

	handshakeBuf := make([]byte, 67)
	_, err = io.ReadFull(r, handshakeBuf)
	if err != nil {
		return nil, fmt.Errorf("reading handshake payload: %w", err)
	}

	pstr := string(handshakeBuf[:pstrlen])
	if pstr != ProtocolString {
		return nil, fmt.Errorf("unsupported protocol: %q", pstr)
	}

	var infoHash [20]byte
	var peerID [20]byte

	copy(infoHash[:], handshakeBuf[pstrlen+8:pstrlen+8+20])
	copy(peerID[:], handshakeBuf[pstrlen+8+20:])

	return &Handshake{
		Pstr:     pstr,
		InfoHash: infoHash,
		PeerID:   peerID,
	}, nil
}
