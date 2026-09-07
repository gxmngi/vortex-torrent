package torrentfile

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"os"

	"github.com/gxmngi/vortex-torrent/bencode"
	"github.com/gxmngi/vortex-torrent/p2p"
)

// TorrentFile represents the parsed metadata of a .torrent file.
type TorrentFile struct {
	Announce    string
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

// Open reads and parses a .torrent file from the specified path.
func Open(path string) (*TorrentFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading torrent file: %w", err)
	}
	return Parse(data)
}

// Parse parses the raw bytes of a .torrent file.
func Parse(data []byte) (*TorrentFile, error) {
	dec := bencode.NewDecoder(data)
	dict, raw, err := dec.DecodeDictWithRaw()
	if err != nil {
		return nil, fmt.Errorf("decoding bencode: %w", err)
	}

	announce, ok := dict["announce"].(string)
	if !ok || announce == "" {
		return nil, errors.New("torrent missing 'announce' URL")
	}

	infoDict, ok := dict["info"].(map[string]interface{})
	if !ok {
		return nil, errors.New("torrent missing 'info' dictionary")
	}

	infoRaw, ok := raw["info"]
	if !ok || len(infoRaw) == 0 {
		return nil, errors.New("torrent missing raw 'info' bytes")
	}

	// Compute 20-byte SHA-1 info_hash from raw bencoded bytes
	infoHash := sha1.Sum(infoRaw)

	name, _ := infoDict["name"].(string)
	if name == "" {
		name = "downloaded_file"
	}

	pieceLen64, ok := infoDict["piece length"].(int64)
	if !ok || pieceLen64 <= 0 {
		return nil, errors.New("invalid 'piece length'")
	}

	length64, ok := infoDict["length"].(int64)
	if !ok || length64 < 0 {
		return nil, errors.New("invalid or unsupported multi-file 'length'")
	}

	piecesStr, ok := infoDict["pieces"].(string)
	if !ok {
		return nil, errors.New("invalid 'pieces' byte sequence")
	}

	piecesBytes := []byte(piecesStr)
	if len(piecesBytes)%20 != 0 {
		return nil, fmt.Errorf("malformed pieces: length %d not multiple of 20", len(piecesBytes))
	}

	numPieces := len(piecesBytes) / 20
	pieceHashes := make([][20]byte, numPieces)
	for i := 0; i < numPieces; i++ {
		copy(pieceHashes[i][:], piecesBytes[i*20:(i+1)*20])
	}

	return &TorrentFile{
		Announce:    announce,
		InfoHash:    infoHash,
		PieceHashes: pieceHashes,
		PieceLength: int(pieceLen64),
		Length:      int(length64),
		Name:        name,
	}, nil
}

// DownloadToFile downloads the torrent to a target path on disk.
func (t *TorrentFile) DownloadToFile(path string) error {
	peerID := GeneratePeerID()

	peersList, err := t.RequestPeers(peerID, 6881)
	if err != nil {
		return fmt.Errorf("requesting peers from tracker: %w", err)
	}

	if len(peersList) == 0 {
		return errors.New("tracker returned 0 active peers in the swarm")
	}

	torrent := p2p.Torrent{
		Peers:       peersList,
		PeerID:      peerID,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
	}

	return torrent.Download(path)
}
