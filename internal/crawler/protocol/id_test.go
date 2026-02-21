package protocol

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestRandomNodeID(t *testing.T) {
	t.Run("returns 20 bytes", func(t *testing.T) {
		const wantLen = 20
		if got := len(RandomNodeID()); got != wantLen {
			t.Fatalf("expected %d bytes, got %d", wantLen, got)
		}
	})

	t.Run("non-zero across multiple calls", func(t *testing.T) {
		// Generate several IDs and confirm at least one differs from the zero
		// value. The probability of a collision with the zero ID is negligible.
		zeroID := ID{}
		allZero := true
		for range 8 {
			if RandomNodeID() != zeroID {
				allZero = false
				break
			}
		}
		if allZero {
			t.Fatal("all generated IDs were zero — crypto/rand is likely broken")
		}
	})

	t.Run("two calls differ", func(t *testing.T) {
		a := RandomNodeID()
		b := RandomNodeID()
		if a == b {
			// Astronomically unlikely; fail loudly if it happens.
			t.Fatalf("two consecutive RandomNodeID calls returned the same value: %s", a)
		}
	})
}

func TestRandomNodeIDWithClientSuffix(t *testing.T) {
	// idClientPart = "-MG0001-" (8 bytes), placed at bytes [12..19] of the 20-byte ID.
	const clientPart = idClientPart

	t.Run("length is 20 bytes", func(t *testing.T) {
		const wantLen = 20
		if got := len(RandomNodeIDWithClientSuffix()); got != wantLen {
			t.Fatalf("expected %d bytes, got %d", wantLen, got)
		}
	})

	t.Run("last N bytes contain client suffix", func(t *testing.T) {
		id := RandomNodeIDWithClientSuffix()
		suffixStart := 20 - len(clientPart)
		got := string(id[suffixStart:])
		if got != clientPart {
			t.Fatalf("expected suffix %q, got %q", clientPart, got)
		}
	})

	t.Run("suffix starts with -MG", func(t *testing.T) {
		id := RandomNodeIDWithClientSuffix()
		suffixStart := 20 - len(clientPart)
		suffix := string(id[suffixStart:])
		if !strings.HasPrefix(suffix, "-MG") {
			t.Fatalf("expected suffix to start with '-MG', got %q", suffix)
		}
	})

	t.Run("prefix bytes are random across calls", func(t *testing.T) {
		a := RandomNodeIDWithClientSuffix()
		b := RandomNodeIDWithClientSuffix()
		suffixStart := 20 - len(clientPart)
		// Convert prefix slices to fixed-size arrays for comparable ==.
		var prefixA, prefixB [12]byte
		copy(prefixA[:], a[:suffixStart])
		copy(prefixB[:], b[:suffixStart])
		if prefixA == prefixB {
			t.Fatal("prefix bytes were identical across two calls — crypto/rand may be broken")
		}
	})
}

func TestID_MarshalBinary_UnmarshalBinary(t *testing.T) {
	tests := []struct {
		name string
		id   ID
	}{
		{
			name: "zero ID",
			id:   ID{},
		},
		{
			name: "all ones",
			id:   [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name: "sequential bytes",
			id:   [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
		},
		{
			name: "random-looking bytes",
			id:   [20]byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x00, 0x11, 0x22, 0x33},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.id.MarshalBinary()
			if err != nil {
				t.Fatalf("MarshalBinary error: %v", err)
			}
			if len(data) != 20 {
				t.Fatalf("MarshalBinary returned %d bytes, want 20", len(data))
			}

			var got ID
			if err := got.UnmarshalBinary(data); err != nil {
				t.Fatalf("UnmarshalBinary error: %v", err)
			}
			if got != tc.id {
				t.Fatalf("roundtrip mismatch: got %v, want %v", got, tc.id)
			}
		})
	}

	t.Run("UnmarshalBinary rejects wrong length", func(t *testing.T) {
		cases := [][]byte{
			{},
			{0x01, 0x02},
			make([]byte, 19),
			make([]byte, 21),
		}
		for _, b := range cases {
			var id ID
			if err := id.UnmarshalBinary(b); err == nil {
				t.Errorf("expected error for %d-byte input, got nil", len(b))
			}
		}
	})
}

func TestID_MarshalJSON_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name    string
		id      ID
		wantHex string
	}{
		{
			name:    "zero ID",
			id:      ID{},
			wantHex: strings.Repeat("0", 40),
		},
		{
			name:    "sequential bytes",
			id:      [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
			wantHex: "000102030405060708090a0b0c0d0e0f10111213",
		},
		{
			name:    "all ff",
			id:      [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			wantHex: strings.Repeat("ff", 20),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := tc.id.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON error: %v", err)
			}

			// The JSON output must be a quoted hex string.
			var s string
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatalf("JSON output is not a valid JSON string: %v", err)
			}
			if s != tc.wantHex {
				t.Fatalf("MarshalJSON hex: got %q, want %q", s, tc.wantHex)
			}

			// Roundtrip via UnmarshalJSON.
			var got ID
			if err := got.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON error: %v", err)
			}
			if got != tc.id {
				t.Fatalf("roundtrip mismatch: got %v, want %v", got, tc.id)
			}
		})
	}

	t.Run("UnmarshalJSON rejects invalid hex", func(t *testing.T) {
		bad := []string{
			`"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"`, // valid length, invalid hex
			`"deadbeef"`,                                  // too short
			`"not-json`,                                   // malformed JSON
		}
		for _, input := range bad {
			var id ID
			if err := id.UnmarshalJSON([]byte(input)); err == nil {
				t.Errorf("expected error for input %s, got nil", input)
			}
		}
	})
}

func TestParseID(t *testing.T) {
	validBytes := [20]byte{0xde, 0xad, 0xbe, 0xef, 0xca, 0xfe, 0xba, 0xbe, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x00, 0x11, 0x22, 0x33}
	validHex := hex.EncodeToString(validBytes[:])

	tests := []struct {
		name    string
		input   string
		wantID  ID
		wantErr bool
	}{
		{
			name:    "valid plain hex",
			input:   validHex,
			wantID:  validBytes,
			wantErr: false,
		},
		{
			name:    "valid hex with 0x prefix",
			input:   "0x" + validHex,
			wantID:  validBytes,
			wantErr: false,
		},
		{
			name:    "all zeros",
			input:   strings.Repeat("0", 40),
			wantID:  ID{},
			wantErr: false,
		},
		{
			name:    "all ff",
			input:   strings.Repeat("ff", 20),
			wantID:  [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			wantErr: false,
		},
		{
			name:    "invalid hex characters",
			input:   strings.Repeat("gg", 20),
			wantErr: true,
		},
		{
			name:    "too short (38 hex chars = 19 bytes)",
			input:   strings.Repeat("aa", 19),
			wantErr: true,
		},
		{
			name:    "too long (42 hex chars = 21 bytes)",
			input:   strings.Repeat("bb", 21),
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "odd-length hex",
			input:   "abc",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseID(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (id=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantID {
				t.Fatalf("got %v, want %v", got, tc.wantID)
			}
		})
	}
}

func TestID_String(t *testing.T) {
	tests := []struct {
		name string
		id   ID
		want string
	}{
		{
			name: "zero ID is 40 zeros",
			id:   ID{},
			want: strings.Repeat("0", 40),
		},
		{
			name: "sequential bytes",
			id:   [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
			want: "000102030405060708090a0b0c0d0e0f10111213",
		},
		{
			name: "all ff",
			id:   [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			want: strings.Repeat("ff", 20),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.id.String()
			if got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
			// String must be exactly 40 hex characters.
			if len(got) != 40 {
				t.Fatalf("String() length = %d, want 40", len(got))
			}
			// hex.EncodeToString always produces lowercase; verify no uppercase.
			if got != strings.ToLower(got) {
				t.Fatalf("String() returned non-lowercase hex: %q", got)
			}
		})
	}

	t.Run("String is valid hex decodable back to original", func(t *testing.T) {
		id := RandomNodeID()
		s := id.String()
		decoded, err := hex.DecodeString(s)
		if err != nil {
			t.Fatalf("String() produced invalid hex: %v", err)
		}
		var roundtripped ID
		copy(roundtripped[:], decoded)
		if roundtripped != id {
			t.Fatalf("roundtrip via String failed: got %v, want %v", roundtripped, id)
		}
	})
}
