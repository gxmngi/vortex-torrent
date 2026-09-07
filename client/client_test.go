package client

import (
	"crypto/sha1"
	"net"
	"testing"

	"github.com/gxmngi/vortex-torrent/message"
	"github.com/gxmngi/vortex-torrent/peers"
)

func TestClientHandshakeAndMessaging(t *testing.T) {
	// Start a mock peer server on loopback
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	mockPeer := peers.Peer{IP: addr.IP, Port: uint16(addr.Port)}

	infoHash := sha1.Sum([]byte("sample-torrent-info"))
	peerID := [20]byte{'V', 'T', '0', '1', '0', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'a', 'b', 'c', 'd'}
	serverPeerID := [20]byte{'M', 'O', 'C', 'K', 'P', 'E', 'E', 'R', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'x', 'y'}

	serverDone := make(chan struct{})

	// Mock server routine
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read client handshake
		hs, err := message.ReadHandshake(conn)
		if err != nil {
			t.Errorf("server read handshake error: %v", err)
			return
		}

		// Reply with server handshake
		respHs := message.NewHandshake(hs.InfoHash, serverPeerID)
		_, _ = conn.Write(respHs.Serialize())

		// Send Bitfield message: 2 pieces available (0b11000000)
		bfMsg := message.Message{
			ID:      message.MsgBitfield,
			Payload: []byte{0b11000000},
		}
		_, _ = conn.Write(bfMsg.Serialize())

		// Read client Interested message
		msg, err := message.Read(conn)
		if err != nil || msg == nil || msg.ID != message.MsgInterested {
			t.Errorf("server expected Interested message, got: %v", msg)
			return
		}

		// Send Unchoke message
		unchokeMsg := message.Message{ID: message.MsgUnchoke}
		_, _ = conn.Write(unchokeMsg.Serialize())
	}()

	// Connect client to mock peer
	c, err := New(mockPeer, peerID, infoHash)
	if err != nil {
		t.Fatalf("client connection failed: %v", err)
	}
	defer c.Conn.Close()

	if !c.Bitfield.HasPiece(0) || !c.Bitfield.HasPiece(1) {
		t.Errorf("expected peer to have pieces 0 and 1, got bitfield %v", c.Bitfield)
	}

	// Read unchoke message from server
	msg, err := c.Read()
	if err != nil {
		t.Fatalf("client read unchoke error: %v", err)
	}
	if msg.ID != message.MsgUnchoke {
		t.Errorf("expected Unchoke, got %v", msg.ID)
	}
	if c.Choked {
		t.Errorf("expected client to be unchoked")
	}

	<-serverDone
}
