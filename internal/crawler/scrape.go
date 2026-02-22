package crawler

import (
	"context"
	"fmt"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
)

// scrapeMaxAttempts is the number of nodes to try before giving up on a BEP 33 scrape.
// Most DHT nodes don't support BEP 33, so we try multiple nodes to increase the chance
// of getting bloom filter data.
const scrapeMaxAttempts = 3

func (c *Crawler) runScrape(ctx context.Context) {
	_ = c.scrape.Run(ctx, func(req nodeHasPeersForHash) {
		pfh, pfhErr := c.requestScrapeWithRetry(ctx, req)
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

// requestScrapeWithRetry tries the original node first, then falls back to
// closest nodes from the routing table if the original doesn't support BEP 33.
func (c *Crawler) requestScrapeWithRetry(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithScrape, error) {
	// Try the original node first
	result, err := c.requestScrape(ctx, req)
	if err == nil {
		return result, nil
	}

	// Get closest nodes from routing table as fallback candidates
	closestNodes := c.kTable.GetClosestNodes(req.infoHash)

	tried := 1
	for _, n := range closestNodes {
		if tried >= scrapeMaxAttempts {
			break
		}
		// Skip the node we already tried
		if n.Addr() == req.node {
			continue
		}
		tried++
		fallbackReq := nodeHasPeersForHash{
			infoHash: req.infoHash,
			node:     n.Addr(),
		}
		result, err = c.requestScrape(ctx, fallbackReq)
		if err == nil {
			return result, nil
		}
	}

	return infoHashWithScrape{}, fmt.Errorf("scrape failed after %d attempts for %x", tried, req.infoHash)
}

func (c *Crawler) requestScrape(
	ctx context.Context,
	req nodeHasPeersForHash,
) (infoHashWithScrape, error) {
	res, err := c.client.GetPeersScrape(ctx, req.node, req.infoHash)
	if err != nil {
		return infoHashWithScrape{}, err
	}
	c.kTable.BatchCommand(ktable.PutNode{
		ID:      res.ID,
		Addr:    req.node,
		Options: []ktable.NodeOption{ktable.NodeResponded()},
	})
	if len(res.Nodes) > 0 {
		cancelCtx, cancel := context.WithTimeout(ctx, time.Second)
	nodeLoop:
		for _, n := range res.Nodes {
			select {
			case <-cancelCtx.Done():
				break nodeLoop
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
