package p2p

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"net"
	"os"
	"testing"

	"github.com/gxmngi/vortex-torrent/message"
	"github.com/gxmngi/vortex-torrent/peers"
)

func TestTorrentConcurrentDownload(t *testing.T) {
	// Create a 32KB synthetic file: 2 pieces of 16384 bytes
	pieceLen := 16384
	fileData := make([]byte, pieceLen*2)
	for i := range fileData {
		fileData[i] = byte(i % 256)
	}

	p0Hash := sha1.Sum(fileData[:pieceLen])
	p1Hash := sha1.Sum(fileData[pieceLen:])
	pieceHashes := [][20]byte{p0Hash, p1Hash}

	infoHash := sha1.Sum([]byte("test-mock-info"))
	peerID := [20]byte{'V', 'T', '0', '1', '0', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'a', 'b', 'c', 'd'}
	serverPeerID := [20]byte{'S', 'E', 'E', 'D', 'E', 'R', '1', '2', '3', '4', '5', '6', '7', '8', '9', '0', 'x', 'y', 'z', 'w'}

	// Start a mock seeder on loopback
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	mockPeer := peers.Peer{IP: addr.IP, Port: uint16(addr.Port)}

	// Mock seeder loop
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Handshake
		hs, err := message.ReadHandshake(conn)
		if err != nil {
			return
		}
		respHs := message.NewHandshake(hs.InfoHash, serverPeerID)
		_, _ = conn.Write(respHs.Serialize())

		// Bitfield (has both pieces 0 and 1)
		bfMsg := message.Message{ID: message.MsgBitfield, Payload: []byte{0b11000000}}
		_, _ = conn.Write(bfMsg.Serialize())

		// Read client messages
		for {
			msg, err := message.Read(conn)
			if err != nil || msg == nil {
				return
			}

			switch msg.ID {
			case message.MsgInterested:
				// Unchoke client immediately
				unchoke := message.Message{ID: message.MsgUnchoke}
				_, _ = conn.Write(unchoke.Serialize())

			case message.MsgRequest:
				index := int(binary.BigEndian.Uint32(msg.Payload[0:4]))
				begin := int(binary.BigEndian.Uint32(msg.Payload[4:8]))
				length := int(binary.BigEndian.Uint32(msg.Payload[8:12]))

				offset := index*pieceLen + begin
				block := fileData[offset : offset+length]

				// Construct Piece message: [4B index][4B begin][data...]
				payload := make([]byte, 8+length)
				binary.BigEndian.PutUint32(payload[0:4], uint32(index))
				binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
				copy(payload[8:], block)

				pieceMsg := message.Message{ID: message.MsgPiece, Payload: payload}
				_, _ = conn.Write(pieceMsg.Serialize())
			}
		}
	}()

	outputPath := "test_output.bin"
	defer func() { _ = os.Remove(outputPath) }()

	torrent := Torrent{
		Peers:       []peers.Peer{mockPeer},
		PeerID:      peerID,
		InfoHash:    infoHash,
		PieceHashes: pieceHashes,
		PieceLength: pieceLen,
		Length:      len(fileData),
		Name:        "test_download",
	}

	err = torrent.Download(outputPath)
	if err != nil {
		t.Fatalf("torrent download failed: %v", err)
	}

	downloadedBytes, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed reading downloaded file: %v", err)
	}

	if !bytes.Equal(downloadedBytes, fileData) {
		t.Errorf("downloaded bytes do not match expected file data")
	}
}
