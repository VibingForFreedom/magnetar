package ktable

import (
	"net/netip"
	"time"
)

const (
	nodesK  = 80
	hashesK = 80
)

// NewTable creates a new Kademlia routing table with the given node ID.
func NewTable(nodeID ID) Table {
	rm := &reverseMap{addrs: make(map[string]*infoForAddr)}
	nodes := nodeKeyspace{
		keyspace: newKeyspace[netip.AddrPort, NodeOption, Node, *node](
			nodeID,
			nodesK,
			func(id ID, addr netip.AddrPort) *node {
				return &node{
					nodeBase: nodeBase{
						id:   id,
						addr: addr,
					},
					discoveredAt: time.Now(),
					reverseMap:   rm,
				}
			},
		),
	}
	hashes := hashKeyspace{
		keyspace: newKeyspace[[]HashPeer, HashOption, Hash, *hash](
			nodeID,
			hashesK,
			func(id ID, peers []HashPeer) *hash {
				peersMap := make(map[string]HashPeer, len(peers))
				for _, p := range peers {
					peersMap[p.Addr.Addr().String()] = p
					rm.putAddrHashes(p.Addr.Addr(), id)
				}
				return &hash{
					id:           id,
					peers:        peersMap,
					discoveredAt: time.Now(),
					reverseMap:   rm,
				}
			},
		),
	}
	return &table{
		origin:  nodeID,
		nodesK:  nodesK,
		hashesK: hashesK,
		nodes:   nodes,
		hashes:  hashes,
		addrs:   rm,
	}
}
