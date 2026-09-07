package bencode

import (
	"bytes"
	"crypto/sha1"
	"reflect"
	"testing"
)

func TestDecodeString(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"4:spam", "spam"},
		{"0:", ""},
		{"12:Hello World!", "Hello World!"},
	}

	for _, c := range cases {
		val, err := Unmarshal([]byte(c.input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", c.input, err)
		}
		if val != c.expected {
			t.Errorf("expected %q, got %q", c.expected, val)
		}
	}
}

func TestDecodeInt(t *testing.T) {
	cases := []struct {
		input    string
		expected int64
	}{
		{"i42e", 42},
		{"i-42e", -42},
		{"i0e", 0},
		{"i1234567890e", 1234567890},
	}

	for _, c := range cases {
		val, err := Unmarshal([]byte(c.input))
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", c.input, err)
		}
		if val != c.expected {
			t.Errorf("expected %d, got %v", c.expected, val)
		}
	}
}

func TestDecodeIntInvalid(t *testing.T) {
	invalid := []string{
		"i03e",  // leading zero
		"i-0e",  // negative zero
		"ie",    // empty
		"i123",  // missing trailing e
	}

	for _, inv := range invalid {
		_, err := Unmarshal([]byte(inv))
		if err == nil {
			t.Errorf("expected error for invalid int %q, got nil", inv)
		}
	}
}

func TestDecodeList(t *testing.T) {
	input := "l4:spami42e4:eggse"
	val, err := Unmarshal([]byte(input))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []interface{}{"spam", int64(42), "eggs"}
	if !reflect.DeepEqual(val, expected) {
		t.Errorf("expected %+v, got %+v", expected, val)
	}
}

func TestDecodeDictAndRawExtraction(t *testing.T) {
	// Sample mock torrent dictionary
	// announce = "http://tracker.com/announce"
	// info = {"length": 1234, "name": "sample.txt"}
	input := []byte("d8:announce27:http://tracker.com/announce4:infod6:lengthi1234e4:name10:sample.txtee")

	dec := NewDecoder(input)
	dict, raw, err := dec.DecodeDictWithRaw()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if dict["announce"] != "http://tracker.com/announce" {
		t.Errorf("unexpected announce: %v", dict["announce"])
	}

	infoRaw, ok := raw["info"]
	if !ok {
		t.Fatalf("expected raw info byte slice")
	}

	expectedRawInfo := []byte("d6:lengthi1234e4:name10:sample.txte")
	if !bytes.Equal(infoRaw, expectedRawInfo) {
		t.Errorf("expected raw info %q, got %q", expectedRawInfo, infoRaw)
	}

	// Verify we can compute SHA-1 InfoHash directly on the raw slice!
	infoHash := sha1.Sum(infoRaw)
	if len(infoHash) != 20 {
		t.Errorf("expected 20-byte sha1 hash, got %d", len(infoHash))
	}
}
