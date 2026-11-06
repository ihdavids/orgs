package excalidraw

import (
	"errors"
	"strings"
	"unicode/utf16"
)

// lz-string's decompressFromBase64 (pieroxy/lz-string 1.5), which is what
// an `.excalidraw.md` keeps a drawing in: the scene's json squeezed with
// LZString.compressToBase64 and broken into lines in a ```compressed-json
// block. The characters it works in are JavaScript's UTF-16 code units, so
// the dictionary holds []uint16 and the answer is decoded from UTF-16 at the
// end (a scene's emoji are surrogate pairs on the way through).

const lzKeyBase64 = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/="

var lzBase64Index = func() [256]int {
	var t [256]int
	for i := range t {
		t[i] = -1
	}
	for i := 0; i < len(lzKeyBase64); i++ {
		t[lzKeyBase64[i]] = i
	}
	return t
}()

var errLzCorrupt = errors.New("the compressed drawing does not decompress")

// DecompressFromBase64 is LZString.decompressFromBase64. Whitespace (the
// line breaks put in every 256 characters) is ignored.
func DecompressFromBase64(in string) (string, error) {
	in = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, in)
	if in == "" {
		return "", errLzCorrupt
	}
	value := func(i int) int {
		if i < 0 || i >= len(in) {
			return 0
		}
		v := lzBase64Index[in[i]]
		if v < 0 {
			return 0
		}
		return v
	}
	out, ok := lzDecompress(len(in), 32, value)
	if !ok {
		return "", errLzCorrupt
	}
	return string(utf16.Decode(out)), nil
}

func lzDecompress(length, resetValue int, next func(int) int) ([]uint16, bool) {
	dict := [][]uint16{{0}, {1}, {2}}
	enlargeIn, dictSize, numBits := 4, 4, 3
	val, position, index := next(0), resetValue, 1

	readBits := func(n int) int {
		bits, power, maxpower := 0, 1, 1<<n
		for power != maxpower {
			resb := val & position
			position >>= 1
			if position == 0 {
				position = resetValue
				val = next(index)
				index++
			}
			if resb > 0 {
				bits |= power
			}
			power <<= 1
		}
		return bits
	}

	var c []uint16
	switch readBits(2) {
	case 0:
		c = []uint16{uint16(readBits(8))}
	case 1:
		c = []uint16{uint16(readBits(16))}
	case 2:
		return nil, true
	}
	dict = append(dict, c)
	w := c
	result := append([]uint16{}, c...)
	for {
		if index > length {
			return nil, false
		}
		code := readBits(numBits)
		switch code {
		case 0, 1:
			n := 8
			if code == 1 {
				n = 16
			}
			dict = append(dict, []uint16{uint16(readBits(n))})
			dictSize++
			code = dictSize - 1
			enlargeIn--
		case 2:
			return result, true
		}
		if enlargeIn == 0 {
			enlargeIn = 1 << numBits
			numBits++
		}
		var entry []uint16
		if code < len(dict) {
			entry = dict[code]
		} else if code == dictSize {
			entry = append(append([]uint16{}, w...), w[0])
		} else {
			return nil, false
		}
		result = append(result, entry...)
		add := append(append([]uint16{}, w...), entry[0])
		dict = append(dict, add)
		dictSize++
		enlargeIn--
		w = entry
		if enlargeIn == 0 {
			enlargeIn = 1 << numBits
			numBits++
		}
	}
}
