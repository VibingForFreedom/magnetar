package metrics

import (
	"testing"
	"time"
)

func TestMetricsCounters(t *testing.T) {
	m := New()

	m.DHTNodesVisited.Add(10)
	m.DHTInfoHashesRecv.Add(5)
	m.TorrentsDiscovered.Add(3)
	m.MetadataFetched.Add(2)
	m.MetadataFailed.Add(1)
	m.MatchAttempts.Add(4)
	m.MatchSuccesses.Add(3)
	m.MatchFailures.Add(1)
	m.TorrentsSaved.Add(2)

	snap := m.Snapshot()

	if snap.DHTNodesVisited != 10 {
		t.Errorf("DHTNodesVisited = %d, want 10", snap.DHTNodesVisited)
	}
	if snap.DHTInfoHashesRecv != 5 {
		t.Errorf("DHTInfoHashesRecv = %d, want 5", snap.DHTInfoHashesRecv)
	}
	if snap.TorrentsDiscovered != 3 {
		t.Errorf("TorrentsDiscovered = %d, want 3", snap.TorrentsDiscovered)
	}
	if snap.MetadataFetched != 2 {
		t.Errorf("MetadataFetched = %d, want 2", snap.MetadataFetched)
	}
	if snap.MetadataFailed != 1 {
		t.Errorf("MetadataFailed = %d, want 1", snap.MetadataFailed)
	}
	if snap.MatchAttempts != 4 {
		t.Errorf("MatchAttempts = %d, want 4", snap.MatchAttempts)
	}
	if snap.MatchSuccesses != 3 {
		t.Errorf("MatchSuccesses = %d, want 3", snap.MatchSuccesses)
	}
	if snap.MatchFailures != 1 {
		t.Errorf("MatchFailures = %d, want 1", snap.MatchFailures)
	}
	if snap.TorrentsSaved != 2 {
		t.Errorf("TorrentsSaved = %d, want 2", snap.TorrentsSaved)
	}
}

func TestMetricsUptime(t *testing.T) {
	m := New()
	m.StartTime = time.Now().Add(-10 * time.Second)

	snap := m.Snapshot()
	if snap.UptimeSeconds < 9 || snap.UptimeSeconds > 11 {
		t.Errorf("UptimeSeconds = %d, want ~10", snap.UptimeSeconds)
	}
}

func TestRateCalcBasic(t *testing.T) {
	rc := NewRateCalc()

	// Record 60 events in the current bucket
	rc.Record(60)

	rate := rc.Rate()
	// 60 events spread over 60-second window = 1.0 events/sec
	if rate < 0.9 || rate > 1.1 {
		t.Errorf("Rate() = %f, want ~1.0", rate)
	}
}

func TestRateCalcZeroWithoutRecords(t *testing.T) {
	rc := NewRateCalc()
	rate := rc.Rate()
	if rate != 0.0 {
		t.Errorf("Rate() = %f, want 0.0", rate)
	}
}

func TestRateCalcAdvanceClearsOldBuckets(t *testing.T) {
	rc := NewRateCalc()
	rc.Record(600) // 600 events in current bucket

	// Simulate 60 seconds passing — all old buckets should be cleared
	rc.mu.Lock()
	rc.advance(time.Now().Add(61 * time.Second))
	rc.mu.Unlock()

	rate := rc.Rate()
	if rate != 0.0 {
		t.Errorf("Rate() = %f after full window advance, want 0.0", rate)
	}
}

func TestRateCalcMultipleBuckets(t *testing.T) {
	rc := NewRateCalc()

	// Record in current bucket
	rc.Record(30)

	// Advance 1 second and record again
	rc.mu.Lock()
	rc.advance(time.Now().Add(1 * time.Second))
	rc.samples[rc.idx] = 30
	rc.mu.Unlock()

	rate := rc.Rate()
	// 60 events over 60 seconds = 1.0
	if rate < 0.9 || rate > 1.1 {
		t.Errorf("Rate() = %f, want ~1.0", rate)
	}
}

func TestMetricsRateRecording(t *testing.T) {
	m := New()

	m.RecordDiscovery(120)
	m.RecordMetadata(60)
	m.RecordMatch(30)

	snap := m.Snapshot()

	if snap.DiscoveryRate < 1.9 || snap.DiscoveryRate > 2.1 {
		t.Errorf("DiscoveryRate = %f, want ~2.0", snap.DiscoveryRate)
	}
	if snap.MetadataRate < 0.9 || snap.MetadataRate > 1.1 {
		t.Errorf("MetadataRate = %f, want ~1.0", snap.MetadataRate)
	}
	if snap.MatchRate < 0.4 || snap.MatchRate > 0.6 {
		t.Errorf("MatchRate = %f, want ~0.5", snap.MatchRate)
	}
}
