package ktable

import (
	"net/netip"
	"sync"
	"time"
)

type hashKeyspace struct {
	keyspace[[]HashPeer, HashOption, Hash, *hash]
}

type Hash interface {
	keyspaceItem
	Peers() []HashPeer
	Dropped() bool
}

type hash struct {
	id           ID
	mu           sync.RWMutex
	peers        map[string]HashPeer
	discoveredAt time.Time
	// lastRequestedAt time.Time
	droppedReason error
	reverseMap    *reverseMap
}

type HashPeer struct {
	Addr netip.AddrPort
}

type HashOption interface {
	apply(*hash)
}

var _ keyspaceItemPrivate[[]HashPeer, HashOption, Hash] = (*hash)(nil)

func (h *hash) update(peers []HashPeer) {
	h.mu.Lock()
	for _, p := range peers {
		h.peers[p.Addr.Addr().String()] = p
		h.reverseMap.putAddrHashes(p.Addr.Addr(), h.id)
	}
	h.mu.Unlock()
}

func (h *hash) apply(option HashOption) {
	option.apply(h)
}

func (h *hash) drop(reason error) {
	h.mu.Lock()
	h.droppedReason = reason
	for _, addr := range h.peers {
		if info, ok := h.reverseMap.addrs[addr.Addr.Addr().String()]; ok {
			info.dropHashes(h.id)
		}
	}
	h.mu.Unlock()
}

func (h *hash) public() Hash {
	return h
}

func (h *hash) ID() ID {
	return h.id
}

func (h *hash) Peers() []HashPeer {
	h.mu.RLock()
	peers := make([]HashPeer, 0, len(h.peers))
	for _, p := range h.peers {
		peers = append(peers, p)
	}
	h.mu.RUnlock()

	return peers
}

func (h *hash) Dropped() bool {
	h.mu.RLock()
	dropped := h.droppedReason != nil
	h.mu.RUnlock()
	return dropped
}
