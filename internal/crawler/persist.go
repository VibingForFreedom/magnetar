package crawler

import (
	"context"
	"time"

	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/crawler/metainfo"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"github.com/magnetar/magnetar/internal/store"
)

// runPersistTorrents receives metadata from the pipeline, classifies it,
// and persists media torrents to the store.
func (c *Crawler) runPersistTorrents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case items := <-c.persistTorrents.Out():
			torrents := make([]*store.Torrent, 0, len(items))
			hashMap := make(map[protocol.ID]infoHashWithMetaInfo, len(items))

			var rejectedHashes [][]byte

			for _, item := range items {
				if _, ok := hashMap[item.infoHash]; ok {
					continue
				}
				hashMap[item.infoHash] = item

				t, ok := c.buildTorrent(item.infoHash, item.metaInfo, item.peerCount)
				if !ok {
					rejectedHashes = append(rejectedHashes, item.infoHash[:])
					continue
				}
				torrents = append(torrents, t)
			}

			// Bulk-insert rejected hashes so triage can skip them on rediscovery
			if len(rejectedHashes) > 0 {
				if err := c.store.RejectHashes(ctx, rejectedHashes); err != nil {
					c.logger.Error("failed to reject hashes", "error", err)
				}
			}

			if len(torrents) == 0 {
				continue
			}

			if err := c.store.UpsertTorrents(ctx, torrents); err != nil {
				c.logger.Error("failed to persist torrents", "error", err)
			} else {
				saved := int64(len(torrents))
				c.metrics.TorrentsDiscovered.Add(saved)
				c.metrics.TorrentsSaved.Add(saved)
				c.metrics.RecordDiscovery(saved)
				c.logger.Debug("persisted torrents", "count", len(torrents))
				// Fire tracker scrapes asynchronously at discovery time
				if c.trackerScraper != nil {
					c.fireTrackerScrapes(ctx, torrents)
				}
				// Forward to scrape channel for S/L counts
				for _, item := range hashMap {
					select {
					case <-ctx.Done():
						return
					case c.scrape.In() <- item.nodeHasPeersForHash:
						continue
					}
				}
			}
		}
	}
}

// buildTorrent converts raw metainfo into a store.Torrent,
// applying classification and media filtering.
// Returns false if the torrent is not media (should be discarded).
func (c *Crawler) buildTorrent(hash protocol.ID, info metainfo.Info, peerCount int) (*store.Torrent, bool) {
	name := info.BestName()

	// Build file list for classification
	classifyFiles := make([]classify.File, 0, len(info.Files))
	storeFiles := make([]store.File, 0, len(info.Files))

	for i, file := range info.Files {
		if i >= int(c.saveFilesThreshold) { //nolint:gosec // value is within range
			break
		}
		path := file.DisplayPath(&info)
		classifyFiles = append(classifyFiles, classify.File{
			Path: path,
			Size: file.Length,
		})
		storeFiles = append(storeFiles, store.File{
			Path: path,
			Size: file.Length,
		})
	}

	// Fast reject: discard software, games, archives, etc.
	if classify.IsJunk(name, classifyFiles) {
		return nil, false
	}

	// Classify the torrent
	cat := classify.Classify(name, classifyFiles)

	// Parse name for metadata extraction
	parsed := classify.Parse(name)
	quality := classify.DetectQuality(name)

	now := time.Now().Unix()

	storeCat := mapCategory(cat)

	// Discard non-media torrents (Unknown category with no media files)
	if storeCat == store.CategoryUnknown && !classify.HasMediaFiles(classifyFiles) {
		return nil, false
	}

	t := &store.Torrent{
		InfoHash:     hash[:],
		Name:         name,
		Size:         info.TotalLength(),
		Category:     storeCat,
		Quality:      mapQuality(quality),
		Files:        storeFiles,
		MediaYear:    parsed.Year,
		IMDBID:       parsed.IMDBID,
		Seeders:      peerCount,
		Source:       store.SourceDHT,
		DiscoveredAt: now,
		UpdatedAt:    now,
	}

	return t, true
}

func mapCategory(c classify.Category) store.Category {
	switch c {
	case classify.CategoryMovie:
		return store.CategoryMovie
	case classify.CategoryTV:
		return store.CategoryTV
	case classify.CategoryAnime:
		return store.CategoryAnime
	default:
		return store.CategoryUnknown
	}
}

func mapQuality(q classify.Quality) store.Quality {
	switch q {
	case classify.QualitySD:
		return store.QualitySD
	case classify.QualityHD:
		return store.QualityHD
	case classify.QualityFHD:
		return store.QualityFHD
	case classify.QualityUHD:
		return store.QualityUHD
	default:
		return store.QualityUnknown
	}
}

// fireTrackerScrapes performs a batch tracker scrape for all newly persisted
// torrents and bulk-updates seeders/leechers counts in a single DB call.
func (c *Crawler) fireTrackerScrapes(ctx context.Context, torrents []*store.Torrent) {
	hashes := make([][20]byte, len(torrents))
	hashToTorrent := make(map[[20]byte]*store.Torrent, len(torrents))
	for i, t := range torrents {
		var h [20]byte
		copy(h[:], t.InfoHash)
		hashes[i] = h
		hashToTorrent[h] = t
	}

	c.metrics.TrackerScrapeAttempts.Add(int64(len(hashes)))
	c.metrics.RecordTrackerScrape(int64(len(hashes)))

	go func() {
		results := c.trackerScraper.ScrapeBatch(ctx, hashes)

		var updates []store.SeedersLeechersUpdate
		for h, r := range results {
			if r.Seeders == 0 && r.Leechers == 0 {
				c.metrics.TrackerScrapeFailures.Add(1)
				continue
			}
			c.metrics.TrackerScrapeSuccesses.Add(1)

			t := hashToTorrent[h]
			if r.Seeders > t.Seeders || r.Leechers > t.Leechers {
				seeders := t.Seeders
				if r.Seeders > seeders {
					seeders = r.Seeders
				}
				leechers := t.Leechers
				if r.Leechers > leechers {
					leechers = r.Leechers
				}
				updates = append(updates, store.SeedersLeechersUpdate{
					InfoHash: t.InfoHash,
					Seeders:  seeders,
					Leechers: leechers,
				})
			}
		}

		// Count hashes that got no result at all as failures
		noResult := len(hashes) - len(results)
		if noResult > 0 {
			c.metrics.TrackerScrapeFailures.Add(int64(noResult))
		}

		if len(updates) > 0 {
			if err := c.store.BulkUpdateSeedersLeechers(ctx, updates); err != nil {
				c.logger.Error("failed to bulk update tracker counts", "error", err)
			} else {
				c.metrics.TrackerScrapeUpdated.Add(int64(len(updates)))
			}
		}
	}()
}

// runPersistSources receives scrape results and updates seeders/leechers in bulk.
func (c *Crawler) runPersistSources(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case scrapes := <-c.persistSources.Out():
			updates := make([]store.SeedersLeechersUpdate, 0, len(scrapes))
			for _, s := range scrapes {
				updates = append(updates, store.SeedersLeechersUpdate{
					InfoHash: s.infoHash[:],
					Seeders:  int(s.bfsd.ApproximatedSize()),
					Leechers: int(s.bfpe.ApproximatedSize()),
				})
			}
			if err := c.store.BulkUpdateSeedersLeechers(ctx, updates); err != nil {
				c.logger.Error("failed to bulk update torrent sources", "error", err)
			}
			c.logger.Debug("persisted torrent sources", "count", len(scrapes))
		}
	}
}
