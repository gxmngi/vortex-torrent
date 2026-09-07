package peers

import (
	"net"
	"testing"
)

func TestUnmarshalPeers(t *testing.T) {
	// Two compact peers:
	// 127.0.0.1:6881 (0x1AE1)
	// 192.168.1.50:8080 (0x1F90)
	input := []byte{
		127, 0, 0, 1, 0x1A, 0xE1,
		192, 168, 1, 50, 0x1F, 0x90,
	}

	peers, err := Unmarshal(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}

	if !peers[0].IP.Equal(net.IPv4(127, 0, 0, 1)) || peers[0].Port != 6881 {
		t.Errorf("peer 0 mismatch: %v", peers[0])
	}
	if !peers[1].IP.Equal(net.IPv4(192, 168, 1, 50)) || peers[1].Port != 8080 {
		t.Errorf("peer 1 mismatch: %v", peers[1])
	}

	if peers[0].String() != "127.0.0.1:6881" {
		t.Errorf("expected 127.0.0.1:6881, got %s", peers[0].String())
	}
}

func TestUnmarshalMalformedPeers(t *testing.T) {
	malformed := []byte{127, 0, 0, 1, 0x1A} // only 5 bytes
	_, err := Unmarshal(malformed)
	if err == nil {
		t.Error("expected error for malformed peer bytes, got nil")
	}
}
