package crockford

import (
	"bytes"
	"strings"
	"testing"
)

func TestAlphabetCorrectness(t *testing.T) {
	if len(alphabet) != 32 {
		t.Fatalf("alphabet length = %d, want 32", len(alphabet))
	}
	for _, excluded := range "ILOU" {
		if strings.ContainsRune(alphabet, excluded) {
			t.Errorf("alphabet should not contain %q", excluded)
		}
	}
}

func TestEncode_KnownVectors(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		want string
	}{
		{"empty", nil, ""},
		{"zero byte", []byte{0x00}, "00"},
		{"max byte", []byte{0xFF}, "ZW"},
		{"two bytes 0x00 0x00", []byte{0x00, 0x00}, "0000"},
		{"0x61 (ASCII 'a')", []byte{0x61}, "C4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Encode(tc.src)
			if got != tc.want {
				t.Errorf("Encode(%x) = %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

func TestEncodeBits_65Bits(t *testing.T) {
	// 9 bytes = 72 bits; encode only 65 bits → 13 chars.
	src := make([]byte, 9)
	for i := range src {
		src[i] = 0xFF
	}
	got := EncodeBits(src, 65)
	if len(got) != 13 {
		t.Errorf("EncodeBits(9 bytes, 65) length = %d, want 13", len(got))
	}
	// All requested bits are 1. First 13 groups of 5 bits = 11111.
	// 65 bits = 13 * 5 exactly, so all chars should be 'Z' (31).
	for i, c := range got {
		if c != 'Z' {
			t.Errorf("char %d = %q, want 'Z'", i, string(c))
		}
	}
}

func TestEncodeBits_256Bits(t *testing.T) {
	src := make([]byte, 32)
	got := EncodeBits(src, 256)
	if len(got) != 52 {
		t.Errorf("EncodeBits(32 bytes, 256) length = %d, want 52", len(got))
	}
}

func TestEncodeBits_TrailingZeroPad(t *testing.T) {
	// 1 byte (8 bits), encode 8 bits → 2 chars.
	// 0xFF = 11111111 → first 5 bits = 11111 = 31 = 'Z',
	// next 3 bits = 111 + 00 pad = 11100 = 28 = 'W'.
	got := EncodeBits([]byte{0xFF}, 8)
	if got != "ZW" {
		t.Errorf("EncodeBits([0xFF], 8) = %q, want %q", got, "ZW")
	}
}

func TestEncodeBits_PanicOnOverflow(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nBits > src*8")
		}
	}()
	EncodeBits([]byte{0x00}, 9) // 1 byte = 8 bits, requesting 9
}

func TestDecode_KnownVectors(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want []byte
	}{
		{"empty", "", nil},
		{"zero", "00", []byte{0x00, 0x00}},
		{"max", "ZW", []byte{0xFF, 0x00}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Decode(tc.s)
			if err != nil {
				t.Fatalf("Decode(%q) error: %v", tc.s, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Errorf("Decode(%q) = %x, want %x", tc.s, got, tc.want)
			}
		})
	}
}

func TestDecode_CaseInsensitive(t *testing.T) {
	upper, err := Decode("ABCD")
	if err != nil {
		t.Fatalf("Decode upper: %v", err)
	}
	lower, err := Decode("abcd")
	if err != nil {
		t.Fatalf("Decode lower: %v", err)
	}
	if !bytes.Equal(upper, lower) {
		t.Errorf("Decode(ABCD) = %x, Decode(abcd) = %x, want equal", upper, lower)
	}
}

func TestDecode_ConfusableMapping(t *testing.T) {
	// I, i, L, l → 1; O, o → 0.
	for _, c := range []string{"I", "i", "L", "l"} {
		got, err := Decode(c)
		if err != nil {
			t.Fatalf("Decode(%q): %v", c, err)
		}
		want, _ := Decode("1")
		if !bytes.Equal(got, want) {
			t.Errorf("Decode(%q) = %x, want same as Decode(\"1\") = %x", c, got, want)
		}
	}
	for _, c := range []string{"O", "o"} {
		got, err := Decode(c)
		if err != nil {
			t.Fatalf("Decode(%q): %v", c, err)
		}
		want, _ := Decode("0")
		if !bytes.Equal(got, want) {
			t.Errorf("Decode(%q) = %x, want same as Decode(\"0\") = %x", c, got, want)
		}
	}
}

func TestDecode_InvalidChar(t *testing.T) {
	for _, s := range []string{"U", "u", "!", " ", "@"} {
		_, err := Decode(s)
		if err == nil {
			t.Errorf("Decode(%q) should return error", s)
		}
	}
}

func TestDecode_IgnoresHyphens(t *testing.T) {
	plain, err := Decode("ABCD")
	if err != nil {
		t.Fatal(err)
	}
	hyphenated, err := Decode("AB-CD")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, hyphenated) {
		t.Errorf("hyphens should be ignored: %x != %x", plain, hyphenated)
	}
}

func TestRoundTrip(t *testing.T) {
	inputs := [][]byte{
		{},
		{0x00},
		{0xFF},
		{0x00, 0x00, 0x00, 0x00, 0x00},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		{0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF},
		make([]byte, 32),
	}
	// Fill the 32-byte slice with a pattern.
	for i := range inputs[len(inputs)-1] {
		inputs[len(inputs)-1][i] = byte(i)
	}

	for _, src := range inputs {
		encoded := Encode(src)
		decoded, err := Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(Encode(%x)) error: %v", src, err)
		}
		// Decode may produce more bytes than src due to padding.
		// Trim to original length and compare.
		if len(src) == 0 {
			if decoded != nil {
				t.Errorf("Decode(Encode(nil)) = %x, want nil", decoded)
			}
			continue
		}
		if len(decoded) < len(src) {
			t.Fatalf("Decode(Encode(%x)): decoded too short %d < %d",
				src, len(decoded), len(src))
		}
		if !bytes.Equal(decoded[:len(src)], src) {
			t.Errorf("Decode(Encode(%x))[:len] = %x, want %x",
				src, decoded[:len(src)], src)
		}
	}
}
