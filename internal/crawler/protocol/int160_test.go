package protocol

import (
	"testing"
)

// makeInt160 is a test helper that builds an Int160 from a 20-byte literal.
func makeInt160(b [20]byte) Int160 {
	return NewInt160FromByteArray(b)
}

func TestNewInt160FromByteArray(t *testing.T) {
	tests := []struct {
		name  string
		input [20]byte
	}{
		{
			name:  "zero bytes",
			input: [20]byte{},
		},
		{
			name:  "all ones",
			input: [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:  "sequential",
			input: [20]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := NewInt160FromByteArray(tc.input)
			if got.AsByteArray() != tc.input {
				t.Fatalf("AsByteArray() = %v, want %v", got.AsByteArray(), tc.input)
			}
		})
	}
}

func TestInt160_ZeroValue(t *testing.T) {
	t.Run("default zero value is zero", func(t *testing.T) {
		var i Int160
		if !i.IsZero() {
			t.Fatal("default Int160 should be zero")
		}
	})

	t.Run("explicit zero bytes is zero", func(t *testing.T) {
		i := makeInt160([20]byte{})
		if !i.IsZero() {
			t.Fatal("Int160 from all-zero bytes should be zero")
		}
	})

	t.Run("non-zero value is not zero", func(t *testing.T) {
		i := makeInt160([20]byte{0x01})
		if i.IsZero() {
			t.Fatal("Int160 with non-zero byte should not be zero")
		}
	})

	t.Run("single trailing non-zero byte is not zero", func(t *testing.T) {
		var b [20]byte
		b[19] = 0x01
		i := makeInt160(b)
		if i.IsZero() {
			t.Fatal("Int160 with last byte set should not be zero")
		}
	})
}

func TestInt160_Xor(t *testing.T) {
	tests := []struct {
		name    string
		a       [20]byte
		b       [20]byte
		wantXor [20]byte
	}{
		{
			name:    "xor with itself is zero",
			a:       [20]byte{0xde, 0xad, 0xbe, 0xef},
			b:       [20]byte{0xde, 0xad, 0xbe, 0xef},
			wantXor: [20]byte{},
		},
		{
			name:    "xor with zero is identity",
			a:       [20]byte{0x01, 0x02, 0x03},
			b:       [20]byte{},
			wantXor: [20]byte{0x01, 0x02, 0x03},
		},
		{
			name: "xor with all ones flips all bits",
			a:    [20]byte{0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0, 0xf0},
			b:    [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			wantXor: [20]byte{0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f, 0x0f},
		},
		{
			name:    "known single-byte xor",
			a:       [20]byte{0b10101010},
			b:       [20]byte{0b11001100},
			wantXor: [20]byte{0b01100110},
		},
		{
			name:    "commutativity: a xor b == b xor a",
			a:       [20]byte{0xAB, 0xCD, 0xEF},
			b:       [20]byte{0x12, 0x34, 0x56},
			wantXor: [20]byte{0xB9, 0xF9, 0xB9},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ia := makeInt160(tc.a)
			ib := makeInt160(tc.b)
			wantI := makeInt160(tc.wantXor)

			var zero Int160
			got := zero.Xor(ia, ib)
			if got.AsByteArray() != wantI.AsByteArray() {
				t.Fatalf("Xor(%x, %x) = %x, want %x",
					tc.a, tc.b, got.AsByteArray(), tc.wantXor)
			}
		})
	}

	t.Run("xor is commutative", func(t *testing.T) {
		a := makeInt160([20]byte{0x12, 0x34, 0x56, 0x78})
		b := makeInt160([20]byte{0xAB, 0xCD, 0xEF, 0x01})
		var z Int160
		ab := z.Xor(a, b)
		ba := z.Xor(b, a)
		if ab.AsByteArray() != ba.AsByteArray() {
			t.Fatalf("Xor is not commutative: a^b=%x, b^a=%x", ab.AsByteArray(), ba.AsByteArray())
		}
	})
}

func TestInt160_Cmp(t *testing.T) {
	tests := []struct {
		name string
		a    [20]byte
		b    [20]byte
		want int // -1, 0, or 1
	}{
		{
			name: "equal values",
			a:    [20]byte{0x01, 0x02, 0x03},
			b:    [20]byte{0x01, 0x02, 0x03},
			want: 0,
		},
		{
			name: "a less than b, first byte",
			a:    [20]byte{0x00},
			b:    [20]byte{0x01},
			want: -1,
		},
		{
			name: "a greater than b, first byte",
			a:    [20]byte{0x02},
			b:    [20]byte{0x01},
			want: 1,
		},
		{
			name: "differ only in last byte: a < b",
			a:    func() [20]byte { var v [20]byte; v[19] = 0x00; return v }(),
			b:    func() [20]byte { var v [20]byte; v[19] = 0x01; return v }(),
			want: -1,
		},
		{
			name: "differ only in last byte: a > b",
			a:    func() [20]byte { var v [20]byte; v[19] = 0x02; return v }(),
			b:    func() [20]byte { var v [20]byte; v[19] = 0x01; return v }(),
			want: 1,
		},
		{
			name: "zero vs all-ones: zero is less",
			a:    [20]byte{},
			b:    [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			want: -1,
		},
		{
			name: "both zero",
			a:    [20]byte{},
			b:    [20]byte{},
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ia := makeInt160(tc.a)
			ib := makeInt160(tc.b)
			got := ia.Cmp(ib)
			if got != tc.want {
				t.Fatalf("Cmp(%x, %x) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}

	t.Run("antisymmetry: cmp(a,b) == -cmp(b,a) for unequal", func(t *testing.T) {
		a := makeInt160([20]byte{0x10})
		b := makeInt160([20]byte{0x20})
		ab := a.Cmp(b)
		ba := b.Cmp(a)
		if ab == ba {
			t.Fatalf("expected antisymmetry: Cmp(a,b)=%d, Cmp(b,a)=%d", ab, ba)
		}
		if ab+ba != 0 {
			t.Fatalf("expected Cmp(a,b) + Cmp(b,a) == 0, got %d + %d", ab, ba)
		}
	})
}

func TestInt160_BitLen(t *testing.T) {
	tests := []struct {
		name   string
		input  [20]byte
		wantGE int // BitLen must be >= this value
		wantLE int // BitLen must be <= this value
		exact  bool
		exact_ int
	}{
		{
			name:   "zero has bit length 0",
			input:  [20]byte{},
			exact:  true,
			exact_: 0,
		},
		{
			name:   "value 1 has bit length 1",
			input:  func() [20]byte { var b [20]byte; b[19] = 1; return b }(),
			exact:  true,
			exact_: 1,
		},
		{
			name:   "value 2 has bit length 2",
			input:  func() [20]byte { var b [20]byte; b[19] = 2; return b }(),
			exact:  true,
			exact_: 2,
		},
		{
			name:   "0x80 in first byte has bit length 160",
			input:  [20]byte{0x80},
			exact:  true,
			exact_: 160,
		},
		{
			name:   "all ones has bit length 160",
			input:  [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			exact:  true,
			exact_: 160,
		},
		{
			name:   "0x01 in first byte has bit length 153",
			input:  [20]byte{0x01},
			exact:  true,
			exact_: 153, // byte index 0, bit value = 2^152 => BitLen = 153
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			i := makeInt160(tc.input)
			got := i.BitLen()
			if tc.exact && got != tc.exact_ {
				t.Fatalf("BitLen() = %d, want %d", got, tc.exact_)
			}
		})
	}

	t.Run("bit length is non-negative", func(t *testing.T) {
		cases := [][20]byte{
			{},
			{0x01},
			{0xff},
			{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80},
		}
		for _, c := range cases {
			i := makeInt160(c)
			if i.BitLen() < 0 {
				t.Fatalf("BitLen() < 0 for input %x", c)
			}
		}
	})

	t.Run("bit length never exceeds 160", func(t *testing.T) {
		all := [20]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		i := makeInt160(all)
		if bl := i.BitLen(); bl > 160 {
			t.Fatalf("BitLen() = %d, must be <= 160", bl)
		}
	})
}

func TestInt160_Distance(t *testing.T) {
	t.Run("distance to self is zero", func(t *testing.T) {
		a := makeInt160([20]byte{0xAB, 0xCD, 0xEF})
		if d := a.Distance(a); !d.IsZero() {
			t.Fatalf("distance to self should be zero, got %x", d.AsByteArray())
		}
	})

	t.Run("distance is symmetric", func(t *testing.T) {
		a := makeInt160([20]byte{0x12, 0x34})
		b := makeInt160([20]byte{0x56, 0x78})
		ab := a.Distance(b)
		ba := b.Distance(a)
		if ab.AsByteArray() != ba.AsByteArray() {
			t.Fatalf("distance(a,b) != distance(b,a): %x vs %x",
				ab.AsByteArray(), ba.AsByteArray())
		}
	})

	t.Run("known distance calculation", func(t *testing.T) {
		a := makeInt160([20]byte{0x0f})
		b := makeInt160([20]byte{0xf0})
		got := a.Distance(b)
		want := makeInt160([20]byte{0xff})
		if got.AsByteArray() != want.AsByteArray() {
			t.Fatalf("distance = %x, want %x", got.AsByteArray(), want.AsByteArray())
		}
	})
}
