package peers

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// Peer represents the network endpoint of a BitTorrent peer.
type Peer struct {
	IP   net.IP
	Port uint16
}

// String returns the host:port representation of the peer.
func (p Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(int(p.Port)))
}

// Unmarshal parses the compact 6-byte peer format.
// Each peer is represented by 4 bytes of IPv4 address and 2 bytes of Big-Endian port.
func Unmarshal(peersBin []byte) ([]Peer, error) {
	const peerSize = 6
	if len(peersBin)%peerSize != 0 {
		return nil, fmt.Errorf("malformed compact peers: length %d is not a multiple of %d", len(peersBin), peerSize)
	}

	numPeers := len(peersBin) / peerSize
	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		peers[i].IP = net.IP(peersBin[offset : offset+4])
		peers[i].Port = binary.BigEndian.Uint16(peersBin[offset+4 : offset+6])
	}
	return peers, nil
}
