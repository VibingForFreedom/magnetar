package crawler

import (
	"context"
	"net"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
)

func (c *Crawler) reseedBootstrapNodes(ctx context.Context) {
	interval := time.Duration(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(interval):
			for _, strAddr := range c.bootstrapNodes {
				addr, err := net.ResolveUDPAddr("udp", strAddr)
				if err != nil {
					c.logger.Warn("failed to resolve bootstrap node", "addr", strAddr, "error", err)
					continue
				}
				select {
				case <-ctx.Done():
					return
				case c.nodesForPing.In() <- ktable.NewNode(ktable.ID{}, addr.AddrPort()):
					continue
				}
			}
		}
		interval = c.reseedBootstrapNodesInterval
	}
}
