// Package crawler implements a DHT crawler for discovering torrent metadata.
// Extracted and adapted from github.com/bitmagnet-io/bitmagnet (MIT License).
// Key changes: fx/GORM/prometheus replaced with Magnetar Store + classify.
package crawler

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/bloom"
	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/dht/client"
	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
	"github.com/magnetar/magnetar/internal/crawler/dht/server"
	"github.com/magnetar/magnetar/internal/crawler/metainfo"
	"github.com/magnetar/magnetar/internal/crawler/metainfo/banning"
	"github.com/magnetar/magnetar/internal/crawler/metainfo/metainforequester"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
	boom "github.com/tylertreat/BoomFilters"
)

type Crawler struct {
	kTable                       ktable.Table
	server                       server.Server
	client                       client.Client
	metainfoRequester            metainforequester.Requester
	banningChecker               banning.Checker
	bootstrapNodes               []string
	reseedBootstrapNodesInterval time.Duration
	getOldestNodesInterval       time.Duration
	oldPeerThreshold             time.Duration
	discoveredNodes              concurrency.BatchingChannel[ktable.Node]
	nodesForPing                 concurrency.BufferedConcurrentChannel[ktable.Node]
	nodesForFindNode             concurrency.BufferedConcurrentChannel[ktable.Node]
	nodesForSampleInfoHashes     concurrency.BufferedConcurrentChannel[ktable.Node]
	infoHashTriage               concurrency.BatchingChannel[nodeHasPeersForHash]
	getPeers                     concurrency.BufferedConcurrentChannel[nodeHasPeersForHash]
	scrape                       concurrency.BufferedConcurrentChannel[nodeHasPeersForHash]
	requestMetaInfo              concurrency.BufferedConcurrentChannel[infoHashWithPeers]
	persistTorrents              concurrency.BatchingChannel[infoHashWithMetaInfo]
	persistSources               concurrency.BatchingChannel[infoHashWithScrape]
	rescrapeThreshold            time.Duration
	saveFilesThreshold           uint
	store                        store.Store
	metrics                      *metrics.Metrics
	ignoreHashes                 *ignoreHashes
	soughtNodeID                 *concurrency.AtomicValue[protocol.ID]
	stopped                      chan struct{}
	logger                       *slog.Logger
}

func (c *Crawler) start(ctx context.Context) {
	go c.rotateSoughtNodeID(ctx)
	go c.runDiscoveredNodes(ctx)
	go c.runPing(ctx)
	go c.runFindNode(ctx)
	go c.getNodesForFindNode(ctx)
	go c.runSampleInfoHashes(ctx)
	go c.getNodesForSampleInfoHashes(ctx)
	go c.runInfoHashTriage(ctx)
	go c.runGetPeers(ctx)
	go c.runRequestMetaInfo(ctx)
	go c.runScrape(ctx)
	go c.reseedBootstrapNodes(ctx)
	go c.runPersistTorrents(ctx)
	go c.runPersistSources(ctx)
	go c.getOldNodes(ctx)
	<-c.stopped
}

type nodeHasPeersForHash struct {
	infoHash protocol.ID
	node     netip.AddrPort
}

type infoHashWithMetaInfo struct {
	nodeHasPeersForHash
	metaInfo metainfo.Info
}

type infoHashWithPeers struct {
	nodeHasPeersForHash
	peers []netip.AddrPort
}

type infoHashWithScrape struct {
	nodeHasPeersForHash
	bfsd bloom.Filter
	bfpe bloom.Filter
}

type ignoreHashes struct {
	mutex sync.Mutex
	bloom *boom.StableBloomFilter
}

func (i *ignoreHashes) testAndAdd(id protocol.ID) bool {
	i.mutex.Lock()
	defer i.mutex.Unlock()
	return i.bloom.TestAndAdd(id[:])
}

func (c *Crawler) rotateSoughtNodeID(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Second):
			c.soughtNodeID.Set(protocol.RandomNodeID())
		}
	}
}
