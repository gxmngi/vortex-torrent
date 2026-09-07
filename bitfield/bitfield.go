package bitfield

// Bitfield represents the piece availability map of a peer.
// Each bit represents one piece index (bit 7 of byte 0 is piece 0).
type Bitfield []byte

// HasPiece checks if the bitfield contains the given piece index.
func (bf Bitfield) HasPiece(index int) bool {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return false
	}
	return (bf[byteIndex] >> (7 - offset)) & 1 != 0
}

// SetPiece marks a piece as available in the bitfield.
func (bf Bitfield) SetPiece(index int) {
	byteIndex := index / 8
	offset := index % 8
	if byteIndex < 0 || byteIndex >= len(bf) {
		return
	}
	bf[byteIndex] |= 1 << (7 - offset)
}
