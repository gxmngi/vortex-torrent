package p2p

import (
	"bytes"
	"crypto/sha1"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gxmngi/vortex-torrent/client"
	"github.com/gxmngi/vortex-torrent/message"
	"github.com/gxmngi/vortex-torrent/peers"
)

// MaxBlockSize is the standard 16KB block request size in BitTorrent.
const MaxBlockSize = 16384

// MaxBacklog is the number of unacknowledged block requests pipelined to a single peer.
const MaxBacklog = 5

// DefaultEndGameThreshold is the remaining piece count that activates End-Game racing.
const DefaultEndGameThreshold = 2

var errPieceCancelled = errors.New("piece cancelled due to racing peer victory")

// Torrent holds the swarm metadata and active peers required for downloading.
type Torrent struct {
	Peers            []peers.Peer
	PeerID           [20]byte
	InfoHash         [20]byte
	PieceHashes      [][20]byte
	PieceLength      int
	Length           int
	Name             string
	EndGameThreshold int
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
	index         int
	client        *client.Client
	buf           []byte
	downloaded    int
	requested     int
	backlog       int
	pendingBlocks map[int]int // offset begin -> length
}

// PieceCoordinator manages thread-safe piece allocation and triggers End-Game mode.
type PieceCoordinator struct {
	mu               sync.Mutex
	pieces           []*pieceWork
	completed        []bool
	inProgress       []int
	remaining        int
	endGameThreshold int
	results          chan<- *pieceResult
}

func newPieceCoordinator(hashes [][20]byte, sizeFunc func(int) int, threshold int, results chan<- *pieceResult) *PieceCoordinator {
	n := len(hashes)
	pieces := make([]*pieceWork, n)
	for i, h := range hashes {
		pieces[i] = &pieceWork{
			index:  i,
			hash:   h,
			length: sizeFunc(i),
		}
	}

	if threshold <= 0 {
		threshold = DefaultEndGameThreshold
	}

	return &PieceCoordinator{
		pieces:           pieces,
		completed:        make([]bool, n),
		inProgress:       make([]int, n),
		remaining:        n,
		endGameThreshold: threshold,
		results:          results,
	}
}

// PickWork assigns work to an unchoked peer. Enters End-Game racing when pieces lag.
func (pc *PieceCoordinator) PickWork(c *client.Client) *pieceWork {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.remaining == 0 {
		return nil
	}

	// Phase 1: Standard allocation of unstarted pieces
	for i, pw := range pc.pieces {
		if !pc.completed[i] && pc.inProgress[i] == 0 && c.Bitfield.HasPiece(i) {
			pc.inProgress[i]++
			return pw
		}
	}

	// Phase 2: End-Game mode (Redundant racing on lagging in-flight pieces)
	if pc.remaining <= pc.endGameThreshold || pc.allRemainingInProgress() {
		for i, pw := range pc.pieces {
			if !pc.completed[i] && c.Bitfield.HasPiece(i) {
				pc.inProgress[i]++
				log.Printf("[P2P] [End-Game] Redundant request: racing for lagging piece #%d", i)
				return pw
			}
		}
	}

	return nil
}

func (pc *PieceCoordinator) allRemainingInProgress() bool {
	for i := range pc.pieces {
		if !pc.completed[i] && pc.inProgress[i] == 0 {
			return false
		}
	}
	return true
}

func (pc *PieceCoordinator) IsCompleted(index int) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.completed[index]
}

func (pc *PieceCoordinator) IsDone() bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.remaining == 0
}

func (pc *PieceCoordinator) Complete(index int, buf []byte) bool {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.completed[index] {
		return false
	}

	pc.completed[index] = true
	pc.remaining--
	pc.results <- &pieceResult{index: index, buf: buf}
	return true
}

func (pc *PieceCoordinator) Release(index int) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if pc.inProgress[index] > 0 {
		pc.inProgress[index]--
	}
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

// readMessage reads a wire message from the client with a 1-second deadline to allow racing cancellation checks.
func (state *pieceProgress) readMessage() (*message.Message, error) {
	_ = state.client.Conn.SetDeadline(time.Now().Add(1 * time.Second))
	defer func() { _ = state.client.Conn.SetDeadline(time.Time{}) }()
	return state.client.Read()
}

// attemptDownloadPiece downloads all 16KB blocks of a single piece from a peer.
func attemptDownloadPiece(c *client.Client, pw *pieceWork, coord *PieceCoordinator) ([]byte, error) {
	state := pieceProgress{
		index:         pw.index,
		client:        c,
		buf:           make([]byte, pw.length),
		pendingBlocks: make(map[int]int),
	}

	for state.downloaded < pw.length {
		// If another peer already completed this piece in End-Game race, cancel pending blocks and exit
		if coord != nil && coord.IsCompleted(pw.index) {
			for begin, length := range state.pendingBlocks {
				_ = c.SendCancel(pw.index, begin, length)
			}
			return nil, errPieceCancelled
		}

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
				state.pendingBlocks[state.requested] = blockSize
				state.backlog++
				state.requested += blockSize
			}
		}

		msg, err := state.readMessage()
		if err != nil {
			// Check if read timed out (allows responsive cancellation checks)
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if coord != nil && coord.IsCompleted(pw.index) {
					for begin, length := range state.pendingBlocks {
						_ = c.SendCancel(pw.index, begin, length)
					}
					return nil, errPieceCancelled
				}
				continue
			}
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
			offset := message.ParsePieceBegin(msg)
			delete(state.pendingBlocks, offset)
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

// startDownloadWorker connects to a peer and pulls work from the coordinator.
func (t *Torrent) startDownloadWorker(peer peers.Peer, coord *PieceCoordinator) {
	c, err := client.New(peer, t.PeerID, t.InfoHash)
	if err != nil {
		return
	}
	defer func() { _ = c.Conn.Close() }()

	for {
		pw := coord.PickWork(c)
		if pw == nil {
			if coord.IsDone() {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		buf, err := attemptDownloadPiece(c, pw, coord)
		if err != nil {
			coord.Release(pw.index)
			if errors.Is(err, errPieceCancelled) {
				log.Printf("[P2P] [End-Game] Lagging piece #%d cancelled (peer %s)", pw.index, peer.String())
				continue
			}
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			log.Printf("[P2P] Piece %d failed integrity check, re-queuing...", pw.index)
			coord.Release(pw.index)
			continue
		}

		_ = c.SendHave(pw.index)
		coord.Complete(pw.index, buf)
	}
}

// Download coordinates concurrent peer workers to download all pieces and writes to file.
func (t *Torrent) Download(outputPath string) error {
	log.Printf("[P2P] Starting download for: %s (%d bytes, %d pieces)", t.Name, t.Length, len(t.PieceHashes))

	results := make(chan *pieceResult)
	coord := newPieceCoordinator(t.PieceHashes, t.calculatePieceSize, t.EndGameThreshold, results)

	// Spawn a concurrent goroutine worker for every active peer in the swarm
	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, coord)
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

	log.Printf("[P2P] All %d pieces verified. Writing output file: %s", donePieces, outputPath)
	return os.WriteFile(outputPath, buf, 0644)
}
