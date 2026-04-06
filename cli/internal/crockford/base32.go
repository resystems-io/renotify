// SPDX-License-Identifier: MIT
// Copyright (c) 2026 Stewart Gebbie and Resystems IO

// Package crockford implements Crockford Base32 encoding and
// decoding. The alphabet excludes I, L, O, U to avoid visual
// ambiguity. See https://www.crockford.com/base32.html.
package crockford

import (
	"fmt"
	"strings"
)

const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Encode encodes all bytes in src as a Crockford Base32 string.
// The output length is ceil(len(src)*8/5).
func Encode(src []byte) string {
	s, _ := EncodeBits(src, len(src)*8) // cannot fail: nBits == len*8
	return s
}

// EncodeBits encodes exactly nBits from src into Crockford
// Base32. Returns an error if nBits > len(src)*8. Bits beyond
// nBits in the final group are zero-padded.
func EncodeBits(src []byte, nBits int) (string, error) {
	if nBits > len(src)*8 {
		return "", fmt.Errorf("crockford: nBits %d exceeds source length %d bytes", nBits, len(src))
	}
	if nBits == 0 {
		return "", nil
	}

	nChars := (nBits + 4) / 5 // ceil(nBits / 5)
	buf := make([]byte, nChars)
	for i := range nChars {
		// Extract 5 bits starting at bit offset i*5.
		val := extractBits(src, i*5, nBits)
		buf[i] = alphabet[val]
	}
	return string(buf), nil
}

// extractBits reads up to 5 bits from src starting at the given
// bit offset. Bits beyond maxBits are treated as zero.
func extractBits(src []byte, bitOffset, maxBits int) byte {
	var val byte
	for b := range 5 {
		bitPos := bitOffset + b
		if bitPos >= maxBits {
			break
		}
		byteIdx := bitPos / 8
		bitIdx := 7 - (bitPos % 8) // MSB first
		if src[byteIdx]&(1<<uint(bitIdx)) != 0 {
			val |= 1 << uint(4-b)
		}
	}
	return val
}

// decodeMap maps byte values to their 5-bit Crockford values.
// -1 = invalid, -2 = unused sentinel.
var decodeMap [256]int8

func init() {
	for i := range decodeMap {
		decodeMap[i] = -1
	}
	for i, c := range alphabet {
		decodeMap[c] = int8(i)
		decodeMap[byte(strings.ToLower(string(c))[0])] = int8(i)
	}
	// Confusable mappings.
	decodeMap['I'] = 1
	decodeMap['i'] = 1
	decodeMap['L'] = 1
	decodeMap['l'] = 1
	decodeMap['O'] = 0
	decodeMap['o'] = 0
}

// Decode decodes a Crockford Base32 string to bytes. It is
// case-insensitive and maps confusable characters (I/L→1, O→0).
// Hyphens are ignored (Crockford allows them as visual separators).
func Decode(s string) ([]byte, error) {
	// Strip hyphens.
	s = strings.ReplaceAll(s, "-", "")
	if len(s) == 0 {
		return nil, nil
	}

	nBits := len(s) * 5
	out := make([]byte, (nBits+7)/8)

	for i, c := range []byte(s) {
		val := decodeMap[c]
		if val < 0 {
			return nil, fmt.Errorf("crockford: invalid character %q at position %d", c, i)
		}
		// Write 5 bits at bit offset i*5.
		setBits(out, i*5, byte(val))
	}
	return out, nil
}

// setBits writes 5 bits of val into dst starting at bitOffset.
func setBits(dst []byte, bitOffset int, val byte) {
	for b := range 5 {
		bitPos := bitOffset + b
		byteIdx := bitPos / 8
		if byteIdx >= len(dst) {
			break
		}
		bitIdx := 7 - (bitPos % 8)
		if val&(1<<uint(4-b)) != 0 {
			dst[byteIdx] |= 1 << uint(bitIdx)
		}
	}
}
