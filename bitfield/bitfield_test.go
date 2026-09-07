package bitfield

import "testing"

func TestBitfieldHasAndSetPiece(t *testing.T) {
	// Byte 0: 0b10100000 -> pieces 0 and 2 are present
	bf := Bitfield([]byte{0b10100000, 0b00000001})

	if !bf.HasPiece(0) {
		t.Errorf("expected piece 0 to be present")
	}
	if bf.HasPiece(1) {
		t.Errorf("expected piece 1 to be absent")
	}
	if !bf.HasPiece(2) {
		t.Errorf("expected piece 2 to be present")
	}
	if !bf.HasPiece(15) {
		t.Errorf("expected piece 15 to be present")
	}
	if bf.HasPiece(16) {
		t.Errorf("expected piece 16 out of bounds to be false")
	}

	// Set piece 1
	bf.SetPiece(1)
	if !bf.HasPiece(1) {
		t.Errorf("expected piece 1 to be present after SetPiece")
	}
}
