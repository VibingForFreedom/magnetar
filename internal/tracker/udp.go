package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/rand/v2"
	"net"
)

const (
	udpConnectMagic = 0x41727101980
	udpActionConnect = 0
	udpActionScrape  = 2

	// maxUDPBatchSize is the max hashes per UDP scrape request.
	// Header is 16 bytes, each hash is 20 bytes. To fit in a standard
	// ~1500 byte MTU packet: (1500-16)/20 = 74.
	maxUDPBatchSize = 74
)

// scrapeUDPBatch performs a BEP 15 UDP tracker scrape for multiple info hashes.
// Returns a map of hash -> ScrapeEntry for all successfully scraped hashes.
func scrapeUDPBatch(ctx context.Context, host string, hashes [][20]byte) (map[[20]byte]ScrapeEntry, error) {
	if len(hashes) == 0 {
		return nil, nil
	}

	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", host)
	if err != nil {
		return nil, fmt.Errorf("dialing UDP tracker: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return nil, fmt.Errorf("setting deadline: %w", err)
		}
	}

	// Step 1: Connect handshake
	txnID := rand.Uint32() //nolint:gosec // not cryptographic

	connectReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connectReq[0:8], udpConnectMagic)
	binary.BigEndian.PutUint32(connectReq[8:12], udpActionConnect)
	binary.BigEndian.PutUint32(connectReq[12:16], txnID)

	if _, err := conn.Write(connectReq); err != nil {
		return nil, fmt.Errorf("sending connect: %w", err)
	}

	connectResp := make([]byte, 16)
	n, err := conn.Read(connectResp)
	if err != nil {
		return nil, fmt.Errorf("reading connect response: %w", err)
	}
	if n < 16 {
		return nil, fmt.Errorf("connect response too short: %d bytes", n)
	}

	action := binary.BigEndian.Uint32(connectResp[0:4])
	respTxnID := binary.BigEndian.Uint32(connectResp[4:8])
	connectionID := binary.BigEndian.Uint64(connectResp[8:16])

	if action != udpActionConnect {
		return nil, fmt.Errorf("unexpected connect action: %d", action)
	}
	if respTxnID != txnID {
		return nil, fmt.Errorf("transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	// Step 2: Scrape with all hashes
	txnID = rand.Uint32() //nolint:gosec // not cryptographic

	// Request: connection_id(8) + action(4) + txn_id(4) + hash(20)*N
	scrapeReq := make([]byte, 16+20*len(hashes))
	binary.BigEndian.PutUint64(scrapeReq[0:8], connectionID)
	binary.BigEndian.PutUint32(scrapeReq[8:12], udpActionScrape)
	binary.BigEndian.PutUint32(scrapeReq[12:16], txnID)
	for i, h := range hashes {
		copy(scrapeReq[16+20*i:16+20*(i+1)], h[:])
	}

	if _, err := conn.Write(scrapeReq); err != nil {
		return nil, fmt.Errorf("sending scrape: %w", err)
	}

	// Response: action(4) + txn_id(4) + (seeders(4) + completed(4) + leechers(4))*N
	expectedSize := 8 + 12*len(hashes)
	scrapeResp := make([]byte, expectedSize)
	n, err = conn.Read(scrapeResp)
	if err != nil {
		return nil, fmt.Errorf("reading scrape response: %w", err)
	}
	if n < 8 {
		return nil, fmt.Errorf("scrape response too short: %d bytes", n)
	}

	action = binary.BigEndian.Uint32(scrapeResp[0:4])
	respTxnID = binary.BigEndian.Uint32(scrapeResp[4:8])

	if action != udpActionScrape {
		return nil, fmt.Errorf("unexpected scrape action: %d", action)
	}
	if respTxnID != txnID {
		return nil, fmt.Errorf("scrape transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	// Parse N entries from response
	result := make(map[[20]byte]ScrapeEntry, len(hashes))
	numEntries := (n - 8) / 12
	for i := range numEntries {
		if i >= len(hashes) {
			break
		}
		offset := 8 + 12*i
		if offset+12 > n {
			break
		}
		seeders := int(binary.BigEndian.Uint32(scrapeResp[offset : offset+4]))
		// bytes offset+4:offset+8 are "completed" (downloads) — skip
		leechers := int(binary.BigEndian.Uint32(scrapeResp[offset+8 : offset+12]))
		result[hashes[i]] = ScrapeEntry{
			Complete:   seeders,
			Incomplete: leechers,
		}
	}

	return result, nil
}

// scrapeUDP performs a BEP 15 UDP tracker scrape for a single info hash.
func scrapeUDP(ctx context.Context, host string, infoHash [20]byte) (ScrapeEntry, error) {
	results, err := scrapeUDPBatch(ctx, host, [][20]byte{infoHash})
	if err != nil {
		return ScrapeEntry{}, err
	}
	if entry, ok := results[infoHash]; ok {
		return entry, nil
	}
	return ScrapeEntry{}, fmt.Errorf("info hash not found in scrape response")
}
