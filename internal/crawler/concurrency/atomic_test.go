package concurrency

import (
	"sync"
	"testing"
)

func TestAtomicValue_SetGet(t *testing.T) {
	t.Run("default zero value for int", func(t *testing.T) {
		var a AtomicValue[int]
		if got := a.Get(); got != 0 {
			t.Fatalf("expected 0, got %d", got)
		}
	})

	t.Run("default zero value for string", func(t *testing.T) {
		var a AtomicValue[string]
		if got := a.Get(); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
	})

	t.Run("Set then Get returns same value for int", func(t *testing.T) {
		var a AtomicValue[int]
		a.Set(42)
		if got := a.Get(); got != 42 {
			t.Fatalf("expected 42, got %d", got)
		}
	})

	t.Run("Set then Get returns same value for string", func(t *testing.T) {
		var a AtomicValue[string]
		a.Set("hello")
		if got := a.Get(); got != "hello" {
			t.Fatalf("expected 'hello', got %q", got)
		}
	})

	t.Run("multiple Sets: last write wins", func(t *testing.T) {
		var a AtomicValue[int]
		a.Set(1)
		a.Set(2)
		a.Set(3)
		if got := a.Get(); got != 3 {
			t.Fatalf("expected 3, got %d", got)
		}
	})

	t.Run("Set to zero value", func(t *testing.T) {
		var a AtomicValue[int]
		a.Set(99)
		a.Set(0)
		if got := a.Get(); got != 0 {
			t.Fatalf("expected 0 after reset, got %d", got)
		}
	})

	t.Run("works with struct type", func(t *testing.T) {
		type point struct{ X, Y int }
		var a AtomicValue[point]
		p := point{X: 3, Y: 7}
		a.Set(p)
		got := a.Get()
		if got != p {
			t.Fatalf("expected %v, got %v", p, got)
		}
	})

	t.Run("works with pointer type", func(t *testing.T) {
		var a AtomicValue[*string]
		s := "magnetar"
		a.Set(&s)
		got := a.Get()
		if got == nil || *got != s {
			t.Fatalf("expected %q, got %v", s, got)
		}
	})
}

func TestAtomicValue_Update(t *testing.T) {
	t.Run("Update transforms value and returns new value", func(t *testing.T) {
		var a AtomicValue[int]
		a.Set(10)
		returned := a.Update(func(v int) int { return v + 5 })
		if returned != 15 {
			t.Fatalf("Update returned %d, want 15", returned)
		}
		if got := a.Get(); got != 15 {
			t.Fatalf("Get after Update = %d, want 15", got)
		}
	})

	t.Run("Update on zero value", func(t *testing.T) {
		var a AtomicValue[int]
		returned := a.Update(func(v int) int { return v + 1 })
		if returned != 1 {
			t.Fatalf("Update returned %d, want 1", returned)
		}
	})

	t.Run("Update with identity function leaves value unchanged", func(t *testing.T) {
		var a AtomicValue[string]
		a.Set("hello")
		returned := a.Update(func(v string) string { return v })
		if returned != "hello" {
			t.Fatalf("expected 'hello', got %q", returned)
		}
	})
}

func TestAtomicValue_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent Set and Get do not race on int", func(t *testing.T) {
		var a AtomicValue[int]
		const goroutines = 50
		const iterations = 100

		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		// Writers.
		for w := range goroutines {
			go func() {
				defer wg.Done()
				for i := range iterations {
					a.Set(w*iterations + i)
				}
			}()
		}

		// Readers — just verify Get does not panic or return a torn value.
		for range goroutines {
			go func() {
				defer wg.Done()
				for range iterations {
					_ = a.Get()
				}
			}()
		}

		wg.Wait()
		// Final Get must complete without error (race detector will catch issues).
		_ = a.Get()
	})

	t.Run("concurrent Updates produce consistent increments", func(t *testing.T) {
		var a AtomicValue[int]
		const goroutines = 20
		const increments = 50

		var wg sync.WaitGroup
		wg.Add(goroutines)

		for range goroutines {
			go func() {
				defer wg.Done()
				for range increments {
					a.Update(func(v int) int { return v + 1 })
				}
			}()
		}

		wg.Wait()

		got := a.Get()
		want := goroutines * increments
		if got != want {
			t.Fatalf("expected %d after concurrent updates, got %d", want, got)
		}
	})

	t.Run("concurrent Set and Get on string do not produce garbage", func(t *testing.T) {
		var a AtomicValue[string]
		// Two valid values that writers alternate between.
		values := [2]string{"alpha", "beta"}
		const goroutines = 30
		const iterations = 80

		var wg sync.WaitGroup
		wg.Add(goroutines * 2)

		for w := range goroutines {
			go func() {
				defer wg.Done()
				for i := range iterations {
					a.Set(values[(w+i)%2])
				}
			}()
		}

		for range goroutines {
			go func() {
				defer wg.Done()
				for range iterations {
					got := a.Get()
					if got != values[0] && got != values[1] && got != "" {
						t.Errorf("unexpected value read: %q", got)
					}
				}
			}()
		}

		wg.Wait()
	})

	t.Run("many readers do not block each other", func(t *testing.T) {
		var a AtomicValue[int]
		a.Set(7)

		const readers = 100
		var wg sync.WaitGroup
		wg.Add(readers)

		for range readers {
			go func() {
				defer wg.Done()
				if got := a.Get(); got != 7 {
					t.Errorf("expected 7, got %d", got)
				}
			}()
		}

		wg.Wait()
	})
}
