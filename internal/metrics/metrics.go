package metrics

import (
	"runtime"
	"sync/atomic"
	"time"
)

// Metrics holds atomic counters for all tracked operations.
type Metrics struct {
	// Crawler counters
	DHTNodesVisited    atomic.Int64
	DHTInfoHashesRecv  atomic.Int64
	TorrentsDiscovered atomic.Int64
	MetadataFetched    atomic.Int64
	MetadataFailed     atomic.Int64

	// Matcher counters
	MatchAttempts  atomic.Int64
	MatchSuccesses atomic.Int64
	MatchFailures  atomic.Int64
	MatchJunk      atomic.Int64

	// Store counters
	TorrentsSaved atomic.Int64

	// Tracker scrape counters
	TrackerScrapeAttempts  atomic.Int64
	TrackerScrapeSuccesses atomic.Int64
	TrackerScrapeFailures  atomic.Int64
	TrackerScrapeUpdated   atomic.Int64

	// Tracker announce counters
	TrackerAnnounceAttempts  atomic.Int64
	TrackerAnnounceSuccesses atomic.Int64

	// getPeers counters
	GetPeersAttempts atomic.Int64
	GetPeersNoPeers  atomic.Int64
	GetPeersFailed   atomic.Int64

	// Rate calculators
	discoveryRate    *RateCalc
	metadataRate     *RateCalc
	matchRate        *RateCalc
	trackerScrapeRate *RateCalc

	// Uptime
	StartTime time.Time
}

// New creates a new Metrics instance with rate calculators initialized.
func New() *Metrics {
	return &Metrics{
		discoveryRate:     NewRateCalc(),
		metadataRate:      NewRateCalc(),
		matchRate:         NewRateCalc(),
		trackerScrapeRate: NewRateCalc(),
		StartTime:         time.Now(),
	}
}

// RecordDiscovery records n torrent discoveries for rate tracking.
func (m *Metrics) RecordDiscovery(n int64) {
	m.discoveryRate.Record(n)
}

// RecordMetadata records n metadata fetches for rate tracking.
func (m *Metrics) RecordMetadata(n int64) {
	m.metadataRate.Record(n)
}

// RecordMatch records n match completions for rate tracking.
func (m *Metrics) RecordMatch(n int64) {
	m.matchRate.Record(n)
}

// RecordTrackerScrape records n tracker scrape completions for rate tracking.
func (m *Metrics) RecordTrackerScrape(n int64) {
	m.trackerScrapeRate.Record(n)
}

// Snapshot returns a point-in-time copy of all metrics.
type Snapshot struct {
	DHTNodesVisited    int64   `json:"dht_nodes_visited"`
	DHTInfoHashesRecv  int64   `json:"dht_info_hashes_recv"`
	TorrentsDiscovered int64   `json:"torrents_discovered"`
	MetadataFetched    int64   `json:"metadata_fetched"`
	MetadataFailed     int64   `json:"metadata_failed"`
	MatchAttempts      int64   `json:"match_attempts"`
	MatchSuccesses     int64   `json:"match_successes"`
	MatchFailures      int64   `json:"match_failures"`
	MatchJunk          int64   `json:"match_junk"`
	TorrentsSaved          int64   `json:"torrents_saved"`
	TrackerScrapeAttempts    int64   `json:"tracker_scrape_attempts"`
	TrackerScrapeSuccesses  int64   `json:"tracker_scrape_successes"`
	TrackerScrapeFailures   int64   `json:"tracker_scrape_failures"`
	TrackerScrapeUpdated    int64   `json:"tracker_scrape_updated"`
	TrackerAnnounceAttempts int64   `json:"tracker_announce_attempts"`
	TrackerAnnounceSuccesses int64  `json:"tracker_announce_successes"`
	GetPeersAttempts        int64   `json:"get_peers_attempts"`
	GetPeersNoPeers         int64   `json:"get_peers_no_peers"`
	GetPeersFailed          int64   `json:"get_peers_failed"`
	DiscoveryRate           float64 `json:"discovery_rate"`
	MetadataRate           float64 `json:"metadata_rate"`
	MatchRate              float64 `json:"match_rate"`
	TrackerScrapeRate      float64 `json:"tracker_scrape_rate"`
	UptimeSeconds          int64   `json:"uptime_seconds"`

	// Runtime memory stats
	HeapInuse    uint64 `json:"heap_inuse"`
	HeapSys      uint64 `json:"heap_sys"`
	HeapReleased uint64 `json:"heap_released"`
	NumGC        uint32 `json:"num_gc"`
	NumGoroutine int    `json:"num_goroutine"`
	MemSysMB     uint64 `json:"mem_sys_mb"`
}

func (m *Metrics) Snapshot() Snapshot {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return Snapshot{
		DHTNodesVisited:    m.DHTNodesVisited.Load(),
		DHTInfoHashesRecv:  m.DHTInfoHashesRecv.Load(),
		TorrentsDiscovered: m.TorrentsDiscovered.Load(),
		MetadataFetched:    m.MetadataFetched.Load(),
		MetadataFailed:     m.MetadataFailed.Load(),
		MatchAttempts:      m.MatchAttempts.Load(),
		MatchSuccesses:     m.MatchSuccesses.Load(),
		MatchFailures:      m.MatchFailures.Load(),
		MatchJunk:          m.MatchJunk.Load(),
		TorrentsSaved:          m.TorrentsSaved.Load(),
		TrackerScrapeAttempts:    m.TrackerScrapeAttempts.Load(),
		TrackerScrapeSuccesses:  m.TrackerScrapeSuccesses.Load(),
		TrackerScrapeFailures:   m.TrackerScrapeFailures.Load(),
		TrackerScrapeUpdated:    m.TrackerScrapeUpdated.Load(),
		TrackerAnnounceAttempts: m.TrackerAnnounceAttempts.Load(),
		TrackerAnnounceSuccesses: m.TrackerAnnounceSuccesses.Load(),
		GetPeersAttempts:        m.GetPeersAttempts.Load(),
		GetPeersNoPeers:         m.GetPeersNoPeers.Load(),
		GetPeersFailed:          m.GetPeersFailed.Load(),
		DiscoveryRate:           m.discoveryRate.Rate(),
		MetadataRate:           m.metadataRate.Rate(),
		MatchRate:              m.matchRate.Rate(),
		TrackerScrapeRate:      m.trackerScrapeRate.Rate(),
		UptimeSeconds:          int64(time.Since(m.StartTime).Seconds()),

		HeapInuse:    mem.HeapInuse,
		HeapSys:      mem.HeapSys,
		HeapReleased: mem.HeapReleased,
		NumGC:        mem.NumGC,
		NumGoroutine: runtime.NumGoroutine(),
		MemSysMB:     mem.Sys / 1024 / 1024,
	}
}
