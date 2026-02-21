package dht

import (
	"crypto/sha1" //nolint:gosec // DHT protocol requires SHA1
	"encoding/binary"
	"math"
	"math/bits"
	"net"

	"github.com/bits-and-blooms/bloom/v3"
)

const (
	scrapeM = 256 * 8
	scrapeK = 2
)

type ScrapeBloomFilter [256]byte

// AddIP adds an IP address to the scrape bloom filter per BEP 33.
func (me *ScrapeBloomFilter) AddIP(ip net.IP) {
	h := sha1.New() //nolint:gosec // DHT protocol requires SHA1
	_, _ = h.Write(ip)
	var sum [20]byte
	h.Sum(sum[:0])
	me.addK(int(sum[0]) | int(sum[1])<<8)
	me.addK(int(sum[2]) | int(sum[3])<<8)
}

func (me *ScrapeBloomFilter) addK(index int) {
	index %= scrapeM
	me[index/8] |= 1 << (index % 8)
}

func (me ScrapeBloomFilter) countZeroes() (ret int) {
	for _, i := range me {
		ret += 8 - bits.OnesCount8(i)
	}
	return
}

func (me *ScrapeBloomFilter) EstimateCount() float64 {
	if me == nil {
		return 0
	}
	c := float64(me.countZeroes())
	if c > scrapeM-1 {
		c = scrapeM - 1
	}
	return math.Log(c/scrapeM) / (scrapeK * math.Log(1.-1./scrapeM))
}

const (
	scrapeBloomSize     = 32
	scrapeBloomByteSize = scrapeBloomSize * 8
	ScrapeBloomM        = scrapeBloomByteSize * 8
	ScrapeBloomK        = 2
)

func (me *ScrapeBloomFilter) ToBloomFilter() *bloom.BloomFilter {
	return bloom.FromWithM(convertScrapeBytes(*me), ScrapeBloomM, ScrapeBloomK)
}

func convertScrapeBytes(b [scrapeBloomByteSize]byte) []uint64 {
	ret := make([]uint64, scrapeBloomSize)
	for i := range scrapeBloomSize {
		startPos := i * 8
		ret[i] = binary.BigEndian.Uint64(b[startPos : startPos+8])
	}
	return ret
}
