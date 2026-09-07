package torrentfile

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"testing"
)

func TestParseTorrentFile(t *testing.T) {
	// Construct a synthetic .torrent file payload
	// 2 pieces: piece 0 hash, piece 1 hash
	p0 := sha1.Sum([]byte("piece-0-data"))
	p1 := sha1.Sum([]byte("piece-1-data"))
	piecesConcat := append(p0[:], p1[:]...)

	rawInfo := fmt.Sprintf("d6:lengthi524288e4:name10:sample.iso12:piece lengthi262144e6:pieces%d:%se", len(piecesConcat), string(piecesConcat))
	expectedInfoHash := sha1.Sum([]byte(rawInfo))

	torrentPayload := fmt.Sprintf("d8:announce27:http://tracker.com/announce4:info%se", rawInfo)

	tf, err := Parse([]byte(torrentPayload))
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if tf.Announce != "http://tracker.com/announce" {
		t.Errorf("expected announce URL, got %s", tf.Announce)
	}
	if tf.Name != "sample.iso" {
		t.Errorf("expected sample.iso, got %s", tf.Name)
	}
	if tf.Length != 524288 {
		t.Errorf("expected 524288, got %d", tf.Length)
	}
	if tf.PieceLength != 262144 {
		t.Errorf("expected 262144, got %d", tf.PieceLength)
	}
	if len(tf.PieceHashes) != 2 {
		t.Fatalf("expected 2 pieces, got %d", len(tf.PieceHashes))
	}
	if !bytes.Equal(tf.PieceHashes[0][:], p0[:]) {
		t.Errorf("piece 0 hash mismatch")
	}
	if !bytes.Equal(tf.PieceHashes[1][:], p1[:]) {
		t.Errorf("piece 1 hash mismatch")
	}
	if !bytes.Equal(tf.InfoHash[:], expectedInfoHash[:]) {
		t.Errorf("info hash mismatch")
	}
}
