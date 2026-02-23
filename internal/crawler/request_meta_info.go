package crawler

import (
	"context"
	"errors"
	"net/netip"
	"sync"

	"github.com/magnetar/magnetar/internal/crawler/metainfo/metainforequester"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

func (c *Crawler) runRequestMetaInfo(ctx context.Context) {
	_ = c.requestMetaInfo.Run(ctx, func(req infoHashWithPeers) {
		if c.paused.Load() {
			return
		}
		mi, reqErr := c.doRequestMetaInfo(ctx, req.infoHash, req.peers)
		if reqErr != nil {
			// Retry once if this was the first attempt and not a banning error.
			if req.retries < 1 {
				retry := infoHashWithPeers{
					nodeHasPeersForHash: req.nodeHasPeersForHash,
					peers:               req.peers,
					retries:             req.retries + 1,
				}
				select {
				case c.metaRetry <- retry:
				default:
					// Retry queue full — count as permanent failure.
					c.metrics.MetadataFailed.Add(1)
				}
			} else {
				c.metrics.MetadataFailed.Add(1)
			}
			return
		}
		c.metrics.MetadataFetched.Add(1)
		c.metrics.RecordMetadata(1)
		select {
		case <-ctx.Done():
		case c.persistTorrents.In() <- infoHashWithMetaInfo{
			nodeHasPeersForHash: req.nodeHasPeersForHash,
			metaInfo:            mi.Info,
			peerCount:           len(req.peers),
		}:
		}
	})
}

func (c *Crawler) doRequestMetaInfo(
	ctx context.Context,
	hash protocol.ID,
	peers []netip.AddrPort,
) (metainforequester.Response, error) {
	// Fan out to up to 5 peers concurrently; first success wins.
	maxConcurrent := 5
	if len(peers) < maxConcurrent {
		maxConcurrent = len(peers)
	}

	type result struct {
		resp metainforequester.Response
		err  error
	}

	fanCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan result, maxConcurrent)
	var errs []error
	var errsMutex sync.Mutex

	for i := range maxConcurrent {
		go func(p netip.AddrPort) {
			res, err := c.metainfoRequester.Request(fanCtx, hash, p)
			if err != nil {
				errsMutex.Lock()
				errs = append(errs, err)
				errsMutex.Unlock()
				results <- result{err: err}
				return
			}
			if banErr := c.banningChecker.Check(res.Info); banErr != nil {
				results <- result{err: banErr}
				return
			}
			results <- result{resp: res}
		}(peers[i])
	}

	for range maxConcurrent {
		r := <-results
		if r.err == nil {
			cancel() // cancel remaining peers
			return r.resp, nil
		}
	}

	return metainforequester.Response{}, errors.Join(errs...)
}
