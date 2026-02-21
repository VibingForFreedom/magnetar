// Package dht implements the BitTorrent DHT protocol types and operations.
// Extracted from github.com/bitmagnet-io/bitmagnet (MIT License).
package dht

import (
	"net/netip"

	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

type ID = protocol.ID

const (
	QPing             = "ping"
	QFindNode         = "find_node"
	QGetPeers         = "get_peers"
	QAnnouncePeer     = "announce_peer"
	QSampleInfohashes = "sample_infohashes"
)

type RecvMsg struct {
	Msg  Msg
	From netip.AddrPort
}

// AnnouncePort returns the torrent port for the message.
// There is an optional argument called implied_port which value is either 0 or 1.
// If it is present and non-zero, the port argument should be ignored and the source port
// of the UDP packet should be used as the peer's port instead.
func (m RecvMsg) AnnouncePort() uint16 {
	port := m.From.Port()
	args := m.Msg.A
	if args != nil && !args.ImpliedPort {
		argsPort := args.Port
		if argsPort != nil {
			port = uint16(*argsPort) //nolint:gosec // value is within range
		}
	}
	return port
}
