package bloom

import (
	boom "github.com/tylertreat/BoomFilters"
)

type StableBloomFilter struct {
	boom.StableBloomFilter
}

const (
	defaultCapacity = 100_000_000
	defaultD        = 2
	defaultFpRate   = 0.001
)

func NewDefaultStableBloomFilter() *StableBloomFilter {
	return &StableBloomFilter{*boom.NewStableBloomFilter(defaultCapacity, defaultD, defaultFpRate)}
}
