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
)

// scrapeUDP performs a BEP 15 UDP tracker scrape for the given info hash.
func scrapeUDP(ctx context.Context, host string, infoHash [20]byte) (ScrapeEntry, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "udp", host)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("dialing UDP tracker: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return ScrapeEntry{}, fmt.Errorf("setting deadline: %w", err)
		}
	}

	// Step 1: Connect handshake
	txnID := rand.Uint32() //nolint:gosec // not cryptographic

	connectReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connectReq[0:8], udpConnectMagic)
	binary.BigEndian.PutUint32(connectReq[8:12], udpActionConnect)
	binary.BigEndian.PutUint32(connectReq[12:16], txnID)

	if _, err := conn.Write(connectReq); err != nil {
		return ScrapeEntry{}, fmt.Errorf("sending connect: %w", err)
	}

	connectResp := make([]byte, 16)
	n, err := conn.Read(connectResp)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("reading connect response: %w", err)
	}
	if n < 16 {
		return ScrapeEntry{}, fmt.Errorf("connect response too short: %d bytes", n)
	}

	action := binary.BigEndian.Uint32(connectResp[0:4])
	respTxnID := binary.BigEndian.Uint32(connectResp[4:8])
	connectionID := binary.BigEndian.Uint64(connectResp[8:16])

	if action != udpActionConnect {
		return ScrapeEntry{}, fmt.Errorf("unexpected connect action: %d", action)
	}
	if respTxnID != txnID {
		return ScrapeEntry{}, fmt.Errorf("transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	// Step 2: Scrape
	txnID = rand.Uint32() //nolint:gosec // not cryptographic

	scrapeReq := make([]byte, 36)
	binary.BigEndian.PutUint64(scrapeReq[0:8], connectionID)
	binary.BigEndian.PutUint32(scrapeReq[8:12], udpActionScrape)
	binary.BigEndian.PutUint32(scrapeReq[12:16], txnID)
	copy(scrapeReq[16:36], infoHash[:])

	if _, err := conn.Write(scrapeReq); err != nil {
		return ScrapeEntry{}, fmt.Errorf("sending scrape: %w", err)
	}

	scrapeResp := make([]byte, 20)
	n, err = conn.Read(scrapeResp)
	if err != nil {
		return ScrapeEntry{}, fmt.Errorf("reading scrape response: %w", err)
	}
	if n < 20 {
		return ScrapeEntry{}, fmt.Errorf("scrape response too short: %d bytes", n)
	}

	action = binary.BigEndian.Uint32(scrapeResp[0:4])
	respTxnID = binary.BigEndian.Uint32(scrapeResp[4:8])

	if action != udpActionScrape {
		return ScrapeEntry{}, fmt.Errorf("unexpected scrape action: %d", action)
	}
	if respTxnID != txnID {
		return ScrapeEntry{}, fmt.Errorf("scrape transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	seeders := int(binary.BigEndian.Uint32(scrapeResp[8:12]))
	// bytes 12:16 are "completed" (downloads) — skip
	leechers := int(binary.BigEndian.Uint32(scrapeResp[16:20]))

	return ScrapeEntry{
		Complete:   seeders,
		Incomplete: leechers,
	}, nil
}
