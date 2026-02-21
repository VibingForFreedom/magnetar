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
			c.metrics.MetadataFailed.Add(1)
			return
		}
		c.metrics.MetadataFetched.Add(1)
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
	var errs []error
	errsMutex := sync.Mutex{}
	addErr := func(err error) {
		errsMutex.Lock()
		errs = append(errs, err)
		errsMutex.Unlock()
	}
	for _, p := range peers {
		res, err := c.metainfoRequester.Request(ctx, hash, p)
		if err != nil {
			addErr(err)
			continue
		}
		if banErr := c.banningChecker.Check(res.Info); banErr != nil {
			// Banned content — just skip, no blocking manager
			return metainforequester.Response{}, banErr
		}
		return res, nil
	}
	return metainforequester.Response{}, errors.Join(errs...)
}
