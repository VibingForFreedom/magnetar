package concurrency

import (
	"testing"
	"time"
)

// drainOut reads all batches from Out() until the channel is closed and returns
// them. Call this after closing In() to wait for the batching goroutine to exit.
func drainOut[T any](ch BatchingChannel[T]) [][]T {
	var result [][]T
	for batch := range ch.Out() {
		result = append(result, batch)
	}
	return result
}

// totalItems counts the total number of items across all batches.
func totalItems[T any](batches [][]T) int {
	n := 0
	for _, b := range batches {
		n += len(b)
	}
	return n
}

// waitForItems waits until at least n items have been received on Out(), or the
// deadline fires. It returns all collected items across batches.
func waitForItems[T any](ch BatchingChannel[T], n int, deadline time.Duration) []T {
	timeout := time.After(deadline)
	var received []T
	for len(received) < n {
		select {
		case batch, ok := <-ch.Out():
			if !ok {
				return received
			}
			received = append(received, batch...)
		case <-timeout:
			return received
		}
	}
	return received
}

// NOTE on design: the batchingChannel goroutine returns (without flushing) when
// In() is closed. Items that have not yet been pushed into a full batch will be
// dropped on shutdown. Tests must therefore either:
//   (a) fill the channel to a batch-size multiple so full batches are flushed
//       before In() is closed, or
//   (b) wait for a time-based flush before closing In().

// --- Items appear in batches ---

func TestBatchingChannel_ItemsAppearInBatches(t *testing.T) {
	t.Run("all items in an exact batch are delivered", func(t *testing.T) {
		// Send exactly maxBatchSize items so they are flushed immediately.
		const (
			capacity     = 64
			maxBatchSize = 5
			maxWait      = 200 * time.Millisecond
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		for i := range maxBatchSize {
			ch.In() <- i
		}

		// Expect the batch to arrive before close.
		batch := <-ch.Out()
		if len(batch) != maxBatchSize {
			t.Fatalf("expected %d items in batch, got %d", maxBatchSize, len(batch))
		}

		close(ch.In())
		drainOut(ch) // wait for goroutine exit
	})

	t.Run("items preserve order within a batch", func(t *testing.T) {
		const (
			capacity     = 32
			maxBatchSize = 5
			maxWait      = 200 * time.Millisecond
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		input := []int{10, 20, 30, 40, 50} // exactly maxBatchSize
		for _, v := range input {
			ch.In() <- v
		}

		batch := <-ch.Out()
		if len(batch) != len(input) {
			t.Fatalf("expected %d items, got %d", len(input), len(batch))
		}
		for i, v := range input {
			if batch[i] != v {
				t.Fatalf("item[%d]: got %d, want %d", i, batch[i], v)
			}
		}

		close(ch.In())
		drainOut(ch)
	})

	t.Run("multiple full batches are all delivered", func(t *testing.T) {
		const (
			capacity     = 128
			maxBatchSize = 4
			maxWait      = 200 * time.Millisecond
			numBatches   = 3
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		total := maxBatchSize * numBatches
		for i := range total {
			ch.In() <- i
		}

		var received []int
		for len(received) < total {
			batch := <-ch.Out()
			received = append(received, batch...)
		}

		close(ch.In())
		drainOut(ch)

		if len(received) != total {
			t.Fatalf("expected %d items, got %d", total, len(received))
		}
		for i, v := range received {
			if v != i {
				t.Fatalf("item[%d]: got %d, want %d", i, v, i)
			}
		}
	})
}

// --- Batch size limit ---

func TestBatchingChannel_BatchSizeLimit(t *testing.T) {
	t.Run("no batch exceeds maxBatchSize", func(t *testing.T) {
		const (
			capacity     = 128
			maxBatchSize = 5
			maxWait      = 200 * time.Millisecond
			numBatches   = 6
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		total := maxBatchSize * numBatches
		for i := range total {
			ch.In() <- i
		}

		var batches [][]int
		var received []int
		for len(received) < total {
			batch := <-ch.Out()
			batches = append(batches, batch)
			received = append(received, batch...)
		}

		close(ch.In())
		drainOut(ch)

		for idx, batch := range batches {
			if len(batch) > maxBatchSize {
				t.Errorf("batch[%d] has %d items, exceeds maxBatchSize=%d", idx, len(batch), maxBatchSize)
			}
		}
	})

	t.Run("a batch of exactly maxBatchSize items is flushed immediately", func(t *testing.T) {
		const (
			capacity     = 16
			maxBatchSize = 3
			maxWait      = 500 * time.Millisecond // long ticker; should not fire
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)
		ch.In() <- 1
		ch.In() <- 2
		ch.In() <- 3

		timeout := time.After(100 * time.Millisecond)
		select {
		case batch := <-ch.Out():
			if len(batch) != maxBatchSize {
				t.Fatalf("expected batch of %d, got %d", maxBatchSize, len(batch))
			}
		case <-timeout:
			t.Fatal("batch of maxBatchSize was not flushed immediately")
		}

		close(ch.In())
		drainOut(ch)
	})
}

// --- Time-based flush ---

func TestBatchingChannel_TimeBasedFlush(t *testing.T) {
	t.Run("partial batch is flushed after maxWaitTime", func(t *testing.T) {
		const (
			capacity     = 16
			maxBatchSize = 100 // large enough that size limit won't trigger
			maxWait      = 40 * time.Millisecond
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		ch.In() <- 1
		ch.In() <- 2
		ch.In() <- 3

		received := waitForItems(ch, 3, 500*time.Millisecond)
		if len(received) != 3 {
			t.Fatalf("expected 3 items from time-based flush, got %d", len(received))
		}

		close(ch.In())
		drainOut(ch)
	})

	t.Run("single item is flushed by ticker", func(t *testing.T) {
		const (
			capacity     = 4
			maxBatchSize = 100
			maxWait      = 30 * time.Millisecond
		)

		ch := NewBatchingChannel[string](capacity, maxBatchSize, maxWait)
		ch.In() <- "hello"

		received := waitForItems(ch, 1, 500*time.Millisecond)
		if len(received) != 1 || received[0] != "hello" {
			t.Fatalf("expected [\"hello\"], got %v", received)
		}

		close(ch.In())
		drainOut(ch)
	})

	t.Run("ticker re-arms after each flush", func(t *testing.T) {
		const (
			capacity     = 16
			maxBatchSize = 50
			maxWait      = 30 * time.Millisecond
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		// First wave.
		ch.In() <- 10
		ch.In() <- 20
		first := waitForItems(ch, 2, 300*time.Millisecond)
		if len(first) < 1 {
			t.Fatal("first time-based flush produced no items")
		}

		// Second wave — ticker must have re-armed.
		time.Sleep(maxWait + 10*time.Millisecond)
		ch.In() <- 30
		ch.In() <- 40
		second := waitForItems(ch, 2, 300*time.Millisecond)
		if len(second) < 1 {
			t.Fatal("second time-based flush produced no items")
		}

		close(ch.In())
		drainOut(ch)
	})

	t.Run("empty channel produces no output before close", func(t *testing.T) {
		const (
			capacity     = 4
			maxBatchSize = 10
			maxWait      = 20 * time.Millisecond
		)

		ch := NewBatchingChannel[int](capacity, maxBatchSize, maxWait)

		// Let three ticker cycles pass without sending anything.
		time.Sleep(3 * maxWait)

		close(ch.In())
		batches := drainOut(ch)
		if totalItems(batches) != 0 {
			t.Fatalf("expected 0 items from empty channel, got %d", totalItems(batches))
		}
	})
}
