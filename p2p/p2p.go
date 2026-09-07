package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gxmngi/vortex-torrent/client"
	"github.com/gxmngi/vortex-torrent/message"
	"github.com/gxmngi/vortex-torrent/peers"
)

// MaxBlockSize is the standard 16KB block request size in BitTorrent.
const MaxBlockSize = 16384

// MaxBacklog is the number of unacknowledged block requests pipelined to a single peer.
const MaxBacklog = 5

// Torrent holds the swarm metadata and active peers required for downloading.
type Torrent struct {
	Peers       []peers.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *client.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (t *Torrent) calculatePieceSize(index int) int {
	begin := index * t.PieceLength
	end := begin + t.PieceLength
	if end > t.Length {
		end = t.Length
	}
	return end - begin
}

func (t *Torrent) calculateBoundsForPiece(index int) (int, int) {
	begin := index * t.PieceLength
	end := begin + t.calculatePieceSize(index)
	return begin, end
}

// readMessageWithTimeout reads a wire message from the client with a 15-second deadline.
func (state *pieceProgress) readMessage() (*message.Message, error) {
	_ = state.client.Conn.SetDeadline(time.Now().Add(15 * time.Second))
	defer func() { _ = state.client.Conn.SetDeadline(time.Time{}) }()
	return state.client.Read()
}

// attemptDownloadPiece downloads all 16KB blocks of a single piece from a peer.
func attemptDownloadPiece(c *client.Client, pw *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pw.index,
		client: c,
		buf:    make([]byte, pw.length),
	}

	for state.downloaded < pw.length {
		// If peer is not choking us, pipeline requests up to MaxBacklog
		if !state.client.Choked {
			for state.backlog < MaxBacklog && state.requested < pw.length {
				blockSize := MaxBlockSize
				if pw.length-state.requested < blockSize {
					blockSize = pw.length - state.requested
				}

				err := c.SendRequest(pw.index, state.requested, blockSize)
				if err != nil {
					return nil, err
				}
				state.backlog++
				state.requested += blockSize
			}
		}

		msg, err := state.readMessage()
		if err != nil {
			return nil, err
		}

		if msg == nil {
			continue // Keep-alive
		}

		switch msg.ID {
		case message.MsgChoke:
			state.client.Choked = true
		case message.MsgUnchoke:
			state.client.Choked = false
		case message.MsgHave:
			index, err := message.ParseHave(msg)
			if err == nil {
				state.client.Bitfield.SetPiece(index)
			}
		case message.MsgPiece:
			n, err := message.ParsePiece(pw.index, state.buf, msg)
			if err != nil {
				return nil, err
			}
			state.downloaded += n
			state.backlog--
		}
	}

	return state.buf, nil
}

// checkIntegrity validates that the SHA-1 hash of the downloaded piece matches the torrent file.
func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("piece %d hash mismatch: expected %x, got %x", pw.index, pw.hash, hash)
	}
	return nil
}

// startDownloadWorker connects to a peer and repeatedly consumes work from the queue.
func (t *Torrent) startDownloadWorker(peer peers.Peer, workQueue chan *pieceWork, results chan *pieceResult) {
	c, err := client.New(peer, t.PeerID, t.InfoHash)
	if err != nil {
		// Peer connection failed or timed out, gracefully return
		return
	}
	defer func() { _ = c.Conn.Close() }()

	for pw := range workQueue {
		if !c.Bitfield.HasPiece(pw.index) {
			workQueue <- pw // Peer doesn't have this piece, return to queue
			time.Sleep(50 * time.Millisecond)
			continue
		}

		buf, err := attemptDownloadPiece(c, pw)
		if err != nil {
			workQueue <- pw // Download failed, return to queue for another peer
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			log.Printf("[P2P] Piece %d failed integrity check, re-queuing...", pw.index)
			workQueue <- pw
			continue
		}

		_ = c.SendHave(pw.index)
		results <- &pieceResult{index: pw.index, buf: buf}
	}
}

// Download coordinates concurrent peer workers to download all pieces and writes to file.
func (t *Torrent) Download(outputPath string) error {
	log.Printf("[P2P] Starting download for: %s (%d bytes, %d pieces)", t.Name, t.Length, len(t.PieceHashes))

	workQueue := make(chan *pieceWork, len(t.PieceHashes))
	results := make(chan *pieceResult)

	for index, hash := range t.PieceHashes {
		workQueue <- &pieceWork{
			index:  index,
			hash:   hash,
			length: t.calculatePieceSize(index),
		}
	}

	// Spawn a concurrent goroutine worker for every active peer in the swarm
	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, results)
	}

	buf := make([]byte, t.Length)
	donePieces := 0

	for donePieces < len(t.PieceHashes) {
		res := <-results
		begin, end := t.calculateBoundsForPiece(res.index)
		copy(buf[begin:end], res.buf)
		donePieces++

		percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
		log.Printf("[P2P] (%d/%d) Downloaded piece #%d [%.1f%% complete]",
			donePieces, len(t.PieceHashes), res.index, percent)
	}

	close(workQueue)

	log.Printf("[P2P] All %d pieces verified. Writing output file: %s", donePieces, outputPath)
	return os.WriteFile(outputPath, buf, 0644)
}
