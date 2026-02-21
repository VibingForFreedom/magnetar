package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/dht/client"
	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
	"github.com/magnetar/magnetar/internal/crawler/dht/responder"
	"github.com/magnetar/magnetar/internal/crawler/dht/server"
	"github.com/magnetar/magnetar/internal/crawler/metainfo/banning"
	"github.com/magnetar/magnetar/internal/crawler/metainfo/metainforequester"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"github.com/magnetar/magnetar/internal/metrics"
	"github.com/magnetar/magnetar/internal/store"
	boom "github.com/tylertreat/BoomFilters"
)

// New creates and returns a new DHT Crawler wired to the given Store.
func New(cfg Config, st store.Store, m *metrics.Metrics, logger *slog.Logger) (*Crawler, error) {
	nodeID := protocol.RandomNodeIDWithClientSuffix()
	kTable := ktable.NewTable(nodeID)
	scalingFactor := int(cfg.ScalingFactor) //nolint:gosec // value is within range

	// Create discovered nodes channel (shared between crawler and responder)
	discoveredNodes := concurrency.NewBatchingChannel[ktable.Node](
		100*scalingFactor, 10, time.Second/100)

	// Build DHT server stack
	serverCfg := server.Config{
		Port:         cfg.Port,
		QueryTimeout: 4 * time.Second,
	}
	resp := responder.New(nodeID, kTable, discoveredNodes.In())
	srv := server.New(serverCfg, resp, logger.With("component", "dht_server"))

	// Build client from server
	cl := client.New(nodeID, srv)

	// Build metadata requester
	miReq := metainforequester.New(metainforequester.NewDefaultConfig())

	c := &Crawler{
		kTable:                       kTable,
		server:                       srv,
		client:                       cl,
		metainfoRequester:            miReq,
		banningChecker:               banning.NewChecker(),
		bootstrapNodes:               cfg.BootstrapNodes,
		reseedBootstrapNodesInterval: cfg.ReseedBootstrapNodesInterval,
		getOldestNodesInterval:       10 * time.Second,
		oldPeerThreshold:             15 * time.Minute,
		discoveredNodes:              discoveredNodes,
		nodesForPing: concurrency.NewBufferedConcurrentChannel[ktable.Node](
			scalingFactor, scalingFactor),
		nodesForFindNode: concurrency.NewBufferedConcurrentChannel[ktable.Node](
			10*scalingFactor, 10*scalingFactor),
		nodesForSampleInfoHashes: concurrency.NewBufferedConcurrentChannel[ktable.Node](
			10*scalingFactor, 10*scalingFactor),
		infoHashTriage: concurrency.NewBatchingChannel[nodeHasPeersForHash](
			10*scalingFactor, 1000, 20*time.Second),
		getPeers: concurrency.NewBufferedConcurrentChannel[nodeHasPeersForHash](
			10*scalingFactor, 20*scalingFactor),
		scrape: concurrency.NewBufferedConcurrentChannel[nodeHasPeersForHash](
			10*scalingFactor, 20*scalingFactor),
		requestMetaInfo: concurrency.NewBufferedConcurrentChannel[infoHashWithPeers](
			10*scalingFactor, 40*scalingFactor),
		persistTorrents: concurrency.NewBatchingChannel[infoHashWithMetaInfo](
			1000, 1000, time.Minute),
		persistSources: concurrency.NewBatchingChannel[infoHashWithScrape](
			1000, 1000, time.Minute),
		saveFilesThreshold: cfg.SaveFilesThreshold,
		rescrapeThreshold:  cfg.RescrapeThreshold,
		store:              st,
		metrics:            m,
		ignoreHashes: &ignoreHashes{
			bloom: boom.NewStableBloomFilter(10_000_000, 2, 0.001),
		},
		soughtNodeID: &concurrency.AtomicValue[protocol.ID]{},
		stopped:      make(chan struct{}),
		logger:       logger.With("component", "dht_crawler"),
	}
	c.soughtNodeID.Set(protocol.RandomNodeID())

	return c, nil
}

// Start begins the crawler pipeline. Blocks until ctx is cancelled.
func (c *Crawler) Start(ctx context.Context) error {
	c.logger.Info("starting DHT crawler")

	// Start the DHT server (opens the UDP socket)
	if err := c.server.Start(); err != nil {
		return fmt.Errorf("starting DHT server: %w", err)
	}

	c.start(ctx)
	return nil
}

// Stop signals the crawler to shut down.
func (c *Crawler) Stop() {
	c.logger.Info("stopping DHT crawler")
	c.server.Stop()
	close(c.stopped)
}
