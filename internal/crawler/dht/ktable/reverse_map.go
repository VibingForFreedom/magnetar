package ktable

import (
	"net/netip"
)

const (
	maxReverseMapAddrs  = 100_000
	maxHashesPerAddr    = 200
)

type reverseMap struct {
	addrs map[string]*infoForAddr
}

type infoForAddr struct {
	peerID ID
	hashes map[ID]struct{}
}

func newInfoForAddr(peerID ID, hashes ...ID) *infoForAddr {
	info := infoForAddr{
		peerID: peerID,
		hashes: make(map[ID]struct{}, len(hashes)),
	}
	info.addHashes(hashes...)

	return &info
}

func (i infoForAddr) addHashes(hashes ...ID) {
	for _, h := range hashes {
		if len(i.hashes) >= maxHashesPerAddr {
			return
		}
		i.hashes[h] = struct{}{}
	}
}

func (i infoForAddr) dropHashes(hashes ...ID) {
	for _, h := range hashes {
		delete(i.hashes, h)
	}
}

func (m reverseMap) putAddrPeerID(addr netip.Addr, id ID) {
	str := addr.String()
	if _, ok := m.addrs[str]; ok {
		m.addrs[str].peerID = id
	} else {
		if len(m.addrs) >= maxReverseMapAddrs {
			m.evictOldest()
		}
		m.addrs[str] = newInfoForAddr(id)
	}
}

func (m reverseMap) putAddrHashes(addr netip.Addr, hashes ...ID) {
	str := addr.String()
	if _, ok := m.addrs[str]; ok {
		m.addrs[str].addHashes(hashes...)
	} else {
		if len(m.addrs) >= maxReverseMapAddrs {
			m.evictOldest()
		}
		m.addrs[str] = newInfoForAddr(ID{}, hashes...)
	}
}

func (m reverseMap) getPeerIDForAddr(addr netip.Addr) (ID, bool) {
	info, ok := m.addrs[addr.String()]
	if ok && !info.peerID.IsZero() {
		return info.peerID, ok
	}

	return ID{}, false
}

func (m reverseMap) dropAddr(addr netip.Addr) {
	delete(m.addrs, addr.String())
}

// evictOldest removes ~10% of entries to make room. Since map iteration order
// is randomized in Go, this effectively removes a random sample.
func (m reverseMap) evictOldest() {
	toEvict := len(m.addrs) / 10
	if toEvict < 1 {
		toEvict = 1
	}
	evicted := 0
	for k := range m.addrs {
		delete(m.addrs, k)
		evicted++
		if evicted >= toEvict {
			break
		}
	}
}
