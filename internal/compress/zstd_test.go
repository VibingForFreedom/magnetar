package compress

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"
)

func TestCompressDecompress(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"simple json", []byte(`{"key":"value","number":42}`)},
		{"array", []byte(`[1,2,3,4,5,6,7,8,9,10]`)},
		{"nested", []byte(`{"a":{"b":{"c":"deep"}}}`)},
		{"unicode", []byte(`{"message":"Hello, 世界!"}`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed, err := Compress(tt.data)
			if err != nil {
				t.Fatalf("Compress() error = %v", err)
			}
			if len(compressed) == 0 {
				t.Fatal("Compress() returned empty result for non-empty input")
			}

			decompressed, err := Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress() error = %v", err)
			}

			if !bytes.Equal(tt.data, decompressed) {
				t.Errorf("roundtrip failed: got %s, want %s", decompressed, tt.data)
			}
		})
	}
}

func TestEmptyInput(t *testing.T) {
	compressed, err := Compress(nil)
	if err != nil {
		t.Fatalf("Compress(nil) error = %v", err)
	}
	if compressed != nil {
		t.Errorf("Compress(nil) = %v, want nil", compressed)
	}

	decompressed, err := Decompress(nil)
	if err != nil {
		t.Fatalf("Decompress(nil) error = %v", err)
	}
	if decompressed != nil {
		t.Errorf("Decompress(nil) = %v, want nil", decompressed)
	}
}

func TestLargeInput(t *testing.T) {
	rand.New(rand.NewSource(42))
	data := make([]byte, 1024*1024)
	rand.Read(data)

	compressed, err := Compress(data)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	if len(compressed) >= len(data) {
		t.Logf("warning: compression ratio >= 1 (input: %d, output: %d)", len(data), len(compressed))
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress() error = %v", err)
	}

	if !bytes.Equal(data, decompressed) {
		t.Error("roundtrip failed for large input")
	}
}

func TestConcurrent(t *testing.T) {
	const goroutines = 10
	const iterations = 100

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				data := []byte(`{"goroutine":id,"iteration":j}`)
				compressed, err := Compress(data)
				if err != nil {
					errCh <- err
					return
				}
				decompressed, err := Decompress(compressed)
				if err != nil {
					errCh <- err
					return
				}
				if !bytes.Equal(data, decompressed) {
					errCh <- fmt.Errorf("mismatch in goroutine %d, iteration %d", id, j)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent test error: %v", err)
	}
}
