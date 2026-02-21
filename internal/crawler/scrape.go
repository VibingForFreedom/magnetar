package crawler

import (
	"context"
	"fmt"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
)

func (c *Crawler) runScrape(ctx context.Context) {
	_ = c.scrape.Run(ctx, func(req nodeHasPeersForHash) {
		pfh, pfhErr := c.requestScrape(ctx, req)
		if pfhErr != nil {
			return
		}
		select {
		case <-ctx.Done():
		case c.persistSources.In() <- infoHashWithScrape{
			nodeHasPeersForHash: req,
			bfsd:                pfh.bfsd,
			bfpe:                pfh.bfpe,
		}:
		}
	})
}

func (c *Crawler) requestScrape(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithScrape, error) {
	res, err := c.client.GetPeersScrape(ctx, req.node, req.infoHash)
	if err != nil {
		c.kTable.BatchCommand(ktable.DropAddr{
			Addr:   req.node.Addr(),
			Reason: fmt.Errorf("failed to get peers from p: %w", err),
		})
		return infoHashWithScrape{}, err
	}
	c.kTable.BatchCommand(ktable.PutNode{
		ID:      res.ID,
		Addr:    req.node,
		Options: []ktable.NodeOption{ktable.NodeResponded()},
	})
	if len(res.Nodes) > 0 {
		cancelCtx, cancel := context.WithTimeout(ctx, time.Second)
		for _, n := range res.Nodes {
			select {
			case <-cancelCtx.Done():
				break
			case c.discoveredNodes.In() <- ktable.NewNode(n.ID, n.Addr):
				continue
			}
		}
		cancel()
	}
	return infoHashWithScrape{
		nodeHasPeersForHash: req,
		bfsd:                res.BfSeeders,
		bfpe:                res.BfPeers,
	}, nil
}
