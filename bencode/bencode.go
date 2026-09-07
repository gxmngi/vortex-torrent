package bencode

import (
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrUnexpectedEOF = errors.New("bencode: unexpected end of data")
	ErrInvalidSyntax = errors.New("bencode: invalid syntax")
)

// Decoder decodes bencoded data into native Go types.
type Decoder struct {
	data []byte
	pos  int
}

// NewDecoder creates a new bencode decoder for the given byte slice.
func NewDecoder(data []byte) *Decoder {
	return &Decoder{data: data, pos: 0}
}

// Decode decodes the next bencode value from data.
func (d *Decoder) Decode() (interface{}, error) {
	if d.pos >= len(d.data) {
		return nil, ErrUnexpectedEOF
	}

	b := d.data[d.pos]
	switch {
	case b >= '0' && b <= '9':
		return d.decodeString()
	case b == 'i':
		return d.decodeInt()
	case b == 'l':
		return d.decodeList()
	case b == 'd':
		val, _, err := d.decodeDict()
		return val, err
	default:
		return nil, fmt.Errorf("%w: unexpected byte '%c' at position %d", ErrInvalidSyntax, b, d.pos)
	}
}

// DecodeDictWithRaw decodes a bencoded dictionary and also returns the raw byte slices
// of each key's value. This is critical for computing the SHA-1 info_hash of the 'info' dict.
func (d *Decoder) DecodeDictWithRaw() (map[string]interface{}, map[string][]byte, error) {
	if d.pos >= len(d.data) {
		return nil, nil, ErrUnexpectedEOF
	}
	if d.data[d.pos] != 'd' {
		return nil, nil, fmt.Errorf("%w: expected 'd' at position %d", ErrInvalidSyntax, d.pos)
	}
	return d.decodeDict()
}

func (d *Decoder) decodeString() (string, error) {
	colonIdx := -1
	for i := d.pos; i < len(d.data); i++ {
		if d.data[i] == ':' {
			colonIdx = i
			break
		}
	}
	if colonIdx == -1 {
		return "", ErrUnexpectedEOF
	}

	lenStr := string(d.data[d.pos:colonIdx])
	strLen, err := strconv.Atoi(lenStr)
	if err != nil || strLen < 0 {
		return "", fmt.Errorf("%w: invalid string length %q", ErrInvalidSyntax, lenStr)
	}

	start := colonIdx + 1
	end := start + strLen
	if end > len(d.data) {
		return "", ErrUnexpectedEOF
	}

	d.pos = end
	return string(d.data[start:end]), nil
}

func (d *Decoder) decodeInt() (int64, error) {
	if d.pos >= len(d.data) || d.data[d.pos] != 'i' {
		return 0, fmt.Errorf("%w: expected 'i' at position %d", ErrInvalidSyntax, d.pos)
	}
	d.pos++ // consume 'i'

	endIdx := -1
	for i := d.pos; i < len(d.data); i++ {
		if d.data[i] == 'e' {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return 0, ErrUnexpectedEOF
	}

	intStr := string(d.data[d.pos:endIdx])
	// In bencode: no leading zeros (except '0'), and no '-0'
	if len(intStr) > 1 && intStr[0] == '0' {
		return 0, fmt.Errorf("%w: leading zero in integer %q", ErrInvalidSyntax, intStr)
	}
	if intStr == "-0" || (len(intStr) > 2 && intStr[:2] == "-0") {
		return 0, fmt.Errorf("%w: negative zero in integer", ErrInvalidSyntax)
	}

	val, err := strconv.ParseInt(intStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid integer %q: %v", ErrInvalidSyntax, intStr, err)
	}

	d.pos = endIdx + 1 // consume 'e'
	return val, nil
}

func (d *Decoder) decodeList() ([]interface{}, error) {
	if d.pos >= len(d.data) || d.data[d.pos] != 'l' {
		return nil, fmt.Errorf("%w: expected 'l' at position %d", ErrInvalidSyntax, d.pos)
	}
	d.pos++ // consume 'l'

	var list []interface{}
	for {
		if d.pos >= len(d.data) {
			return nil, ErrUnexpectedEOF
		}
		if d.data[d.pos] == 'e' {
			d.pos++ // consume 'e'
			break
		}

		elem, err := d.Decode()
		if err != nil {
			return nil, err
		}
		list = append(list, elem)
	}

	return list, nil
}

func (d *Decoder) decodeDict() (map[string]interface{}, map[string][]byte, error) {
	if d.pos >= len(d.data) || d.data[d.pos] != 'd' {
		return nil, nil, fmt.Errorf("%w: expected 'd' at position %d", ErrInvalidSyntax, d.pos)
	}
	d.pos++ // consume 'd'

	dict := make(map[string]interface{})
	rawValues := make(map[string][]byte)

	for {
		if d.pos >= len(d.data) {
			return nil, nil, ErrUnexpectedEOF
		}
		if d.data[d.pos] == 'e' {
			d.pos++ // consume 'e'
			break
		}

		// Key MUST be a bencoded string
		key, err := d.decodeString()
		if err != nil {
			return nil, nil, fmt.Errorf("dict key error: %w", err)
		}

		valStart := d.pos
		val, err := d.Decode()
		if err != nil {
			return nil, nil, fmt.Errorf("dict val error for key %q: %w", key, err)
		}
		valEnd := d.pos

		dict[key] = val
		rawValues[key] = d.data[valStart:valEnd]
	}

	return dict, rawValues, nil
}

// Unmarshal parses the bencoded data and returns the decoded interface value.
func Unmarshal(data []byte) (interface{}, error) {
	return NewDecoder(data).Decode()
}
