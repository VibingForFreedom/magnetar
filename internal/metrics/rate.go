package metrics

import (
	"sync"
	"time"
)

const windowSize = 60

// RateCalc computes an events/sec rate over a sliding window of 60 one-second buckets.
type RateCalc struct {
	mu      sync.Mutex
	samples [windowSize]int64
	idx     int
	last    time.Time
}

// NewRateCalc creates a new rate calculator.
func NewRateCalc() *RateCalc {
	return &RateCalc{
		last: time.Now(),
	}
}

// Record adds n events to the current second's bucket.
func (r *RateCalc) Record(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance(time.Now())
	r.samples[r.idx] += n
}

// Rate returns the average events/sec over the sliding window.
func (r *RateCalc) Rate() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.advance(time.Now())

	var total int64
	for _, s := range r.samples {
		total += s
	}
	return float64(total) / float64(windowSize)
}

// advance moves the index forward, zeroing skipped buckets.
func (r *RateCalc) advance(now time.Time) {
	elapsed := int(now.Sub(r.last).Seconds())
	if elapsed <= 0 {
		return
	}

	// Cap to window size to avoid looping more than necessary
	if elapsed > windowSize {
		elapsed = windowSize
	}

	for i := 0; i < elapsed; i++ {
		r.idx = (r.idx + 1) % windowSize
		r.samples[r.idx] = 0
	}
	r.last = now
}
