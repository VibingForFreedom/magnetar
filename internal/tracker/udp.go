package tracker

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand/v2"
	"net"
	"net/netip"
)

const (
	udpConnectMagic  = 0x41727101980
	udpActionConnect = 0
	udpActionAnnounce = 1
	udpActionScrape  = 2

	// maxUDPBatchSize is the max hashes per UDP scrape request.
	// Header is 16 bytes, each hash is 20 bytes. To fit in a standard
	// ~1500 byte MTU packet: (1500-16)/20 = 74.
	maxUDPBatchSize = 74
)

// udpConnect performs the BEP 15 connect handshake over an existing UDP connection.
// Returns the connection_id on success.
func udpConnect(conn net.Conn) (uint64, error) {
	txnID := rand.Uint32() //nolint:gosec // not cryptographic

	connectReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connectReq[0:8], udpConnectMagic)
	binary.BigEndian.PutUint32(connectReq[8:12], udpActionConnect)
	binary.BigEndian.PutUint32(connectReq[12:16], txnID)

	if _, err := conn.Write(connectReq); err != nil {
		return 0, fmt.Errorf("sending connect: %w", err)
	}

	connectResp := make([]byte, 16)
	n, err := conn.Read(connectResp)
	if err != nil {
		return 0, fmt.Errorf("reading connect response: %w", err)
	}
	if n < 16 {
		return 0, fmt.Errorf("connect response too short: %d bytes", n)
	}

	action := binary.BigEndian.Uint32(connectResp[0:4])
	respTxnID := binary.BigEndian.Uint32(connectResp[4:8])
	connectionID := binary.BigEndian.Uint64(connectResp[8:16])

	if action != udpActionConnect {
		return 0, fmt.Errorf("unexpected connect action: %d", action)
	}
	if respTxnID != txnID {
		return 0, fmt.Errorf("transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	return connectionID, nil
}

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

	connectionID, err := udpConnect(conn)
	if err != nil {
		return nil, err
	}

	// Scrape with all hashes
	txnID := rand.Uint32() //nolint:gosec // not cryptographic

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
	n, err := conn.Read(scrapeResp)
	if err != nil {
		return nil, fmt.Errorf("reading scrape response: %w", err)
	}
	if n < 8 {
		return nil, fmt.Errorf("scrape response too short: %d bytes", n)
	}

	action := binary.BigEndian.Uint32(scrapeResp[0:4])
	respTxnID := binary.BigEndian.Uint32(scrapeResp[4:8])

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

// announceUDP performs a BEP 15 UDP announce to get peer addresses for an info hash.
func announceUDP(ctx context.Context, host string, infoHash [20]byte) ([]netip.AddrPort, error) {
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

	connectionID, err := udpConnect(conn)
	if err != nil {
		return nil, err
	}

	// Build announce request (98 bytes)
	txnID := rand.Uint32() //nolint:gosec // not cryptographic
	req := make([]byte, 98)
	binary.BigEndian.PutUint64(req[0:8], connectionID)      // connection_id
	binary.BigEndian.PutUint32(req[8:12], udpActionAnnounce) // action=1
	binary.BigEndian.PutUint32(req[12:16], txnID)            // transaction_id
	copy(req[16:36], infoHash[:])                            // info_hash

	// peer_id: random 20 bytes
	var peerID [20]byte
	peerID[0] = '-'
	peerID[1] = 'M'
	peerID[2] = 'G'
	for i := 3; i < 20; i++ {
		peerID[i] = byte(rand.IntN(256)) //nolint:gosec
	}
	copy(req[36:56], peerID[:])

	// downloaded=0, left=maxint64, uploaded=0
	binary.BigEndian.PutUint64(req[56:64], 0)                  // downloaded
	binary.BigEndian.PutUint64(req[64:72], math.MaxInt64)      // left
	binary.BigEndian.PutUint64(req[72:80], 0)                  // uploaded
	binary.BigEndian.PutUint32(req[80:84], 0)                  // event (0=none)
	binary.BigEndian.PutUint32(req[84:88], 0)                  // IP (0=default)
	binary.BigEndian.PutUint32(req[88:92], rand.Uint32())      // key //nolint:gosec
	binary.BigEndian.PutUint32(req[92:96], 50)                 // num_want
	binary.BigEndian.PutUint16(req[96:98], 6881)               // port

	if _, err := conn.Write(req); err != nil {
		return nil, fmt.Errorf("sending announce: %w", err)
	}

	// Response: action(4) + txn_id(4) + interval(4) + leechers(4) + seeders(4) + peers(6*N)
	resp := make([]byte, 20+6*50) // header + up to 50 peers
	n, err := conn.Read(resp)
	if err != nil {
		return nil, fmt.Errorf("reading announce response: %w", err)
	}
	if n < 20 {
		return nil, fmt.Errorf("announce response too short: %d bytes", n)
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	respTxnID := binary.BigEndian.Uint32(resp[4:8])

	if action != udpActionAnnounce {
		return nil, fmt.Errorf("unexpected announce action: %d", action)
	}
	if respTxnID != txnID {
		return nil, fmt.Errorf("announce transaction ID mismatch: got %d, want %d", respTxnID, txnID)
	}

	// Parse compact peers: IP(4) + port(2) per peer
	peerData := resp[20:n]
	numPeers := len(peerData) / 6
	peers := make([]netip.AddrPort, 0, numPeers)
	for i := range numPeers {
		off := i * 6
		ip := netip.AddrFrom4([4]byte{peerData[off], peerData[off+1], peerData[off+2], peerData[off+3]})
		port := binary.BigEndian.Uint16(peerData[off+4 : off+6])
		peers = append(peers, netip.AddrPortFrom(ip, port))
	}

	return peers, nil
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
