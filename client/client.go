package client

import (
	"bytes"
	"fmt"
	"net"
	"time"

	"github.com/gxmngi/vortex-torrent/bitfield"
	"github.com/gxmngi/vortex-torrent/message"
	"github.com/gxmngi/vortex-torrent/peers"
)

// Client represents an active TCP connection to a BitTorrent peer.
type Client struct {
	Conn     net.Conn
	Choked   bool
	Bitfield bitfield.Bitfield
	peer     peers.Peer
	infoHash [20]byte
	peerID   [20]byte
}

// completeHandshake performs the 68-byte bidirectional handshake with the peer.
func completeHandshake(conn net.Conn, infoHash, peerID [20]byte) (*message.Handshake, error) {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	req := message.NewHandshake(infoHash, peerID)
	_, err := conn.Write(req.Serialize())
	if err != nil {
		return nil, fmt.Errorf("sending handshake: %w", err)
	}

	res, err := message.ReadHandshake(conn)
	if err != nil {
		return nil, fmt.Errorf("reading handshake: %w", err)
	}

	if !bytes.Equal(res.InfoHash[:], infoHash[:]) {
		return nil, fmt.Errorf("peer info_hash mismatch: expected %x, got %x", infoHash, res.InfoHash)
	}

	return res, nil
}

// recvBitfield waits for the optional initial Bitfield message from the peer.
func recvBitfield(conn net.Conn) (bitfield.Bitfield, error) {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	msg, err := message.Read(conn)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, fmt.Errorf("expected bitfield message, received keep-alive")
	}
	if msg.ID != message.MsgBitfield {
		return nil, fmt.Errorf("expected bitfield (ID %d), got ID %d", message.MsgBitfield, msg.ID)
	}

	return msg.Payload, nil
}

// New connects to a peer, completes the handshake, exchanges initial bitfield, and sends Interested.
func New(peer peers.Peer, peerID, infoHash [20]byte) (*Client, error) {
	conn, err := net.DialTimeout("tcp", peer.String(), 3*time.Second)
	if err != nil {
		return nil, err
	}

	_, err = completeHandshake(conn, infoHash, peerID)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	bf, err := recvBitfield(conn)
	if err != nil {
		// Some peers do not send bitfield if they have no pieces, or send Unchoke directly.
		// We initialize an empty bitfield in that case rather than failing hard.
		bf = make(bitfield.Bitfield, 0)
	}

	c := &Client{
		Conn:     conn,
		Choked:   true,
		Bitfield: bf,
		peer:     peer,
		infoHash: infoHash,
		peerID:   peerID,
	}

	// Announce that we are interested in downloading pieces
	_ = c.SendInterested()

	return c, nil
}

// Read reads a wire message from the peer and updates internal state (such as Have/Choke).
func (c *Client) Read() (*message.Message, error) {
	msg, err := message.Read(c.Conn)
	if err != nil {
		return nil, err
	}

	if msg == nil {
		return nil, nil // KeepAlive
	}

	switch msg.ID {
	case message.MsgUnchoke:
		c.Choked = false
	case message.MsgChoke:
		c.Choked = true
	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err == nil {
			c.Bitfield.SetPiece(index)
		}
	}

	return msg, nil
}

// SendRequest sends a Request message to the peer asking for a block.
func (c *Client) SendRequest(index, begin, length int) error {
	req := message.FormatRequest(index, begin, length)
	_, err := c.Conn.Write(req.Serialize())
	return err
}

// SendInterested announces interest to the peer.
func (c *Client) SendInterested() error {
	msg := message.Message{ID: message.MsgInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

// SendNotInterested announces no interest to the peer.
func (c *Client) SendNotInterested() error {
	msg := message.Message{ID: message.MsgNotInterested}
	_, err := c.Conn.Write(msg.Serialize())
	return err
}

// SendHave informs the peer that we have downloaded a piece.
func (c *Client) SendHave(index int) error {
	msg := message.FormatHave(index)
	_, err := c.Conn.Write(msg.Serialize())
	return err
}
