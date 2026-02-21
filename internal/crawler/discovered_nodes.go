package crawler

import (
	"context"
	"net/netip"

	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
)

func (c *Crawler) runDiscoveredNodes(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ps := <-c.discoveredNodes.Out():
			addrs := make([]netip.Addr, 0, 1)
			m := make(map[string]ktable.Node, 1)
			for _, p := range ps {
				if _, ok := m[p.Addr().Addr().String()]; !ok {
					m[p.Addr().Addr().String()] = p
					addrs = append(addrs, p.Addr().Addr())
				}
			}
			unknownAddrs := c.kTable.FilterKnownAddrs(addrs)
			for _, addr := range unknownAddrs {
				p := m[addr.String()]
				select {
				case <-ctx.Done():
					return
				case c.nodesForFindNode.In() <- p:
				case c.nodesForSampleInfoHashes.In() <- p:
				case c.nodesForPing.In() <- p:
				}
			}
		}
	}
}
