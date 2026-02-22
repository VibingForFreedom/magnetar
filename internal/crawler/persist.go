package crawler

import (
	"context"
	"time"

	"github.com/magnetar/magnetar/internal/classify"
	"github.com/magnetar/magnetar/internal/crawler/metainfo"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"github.com/magnetar/magnetar/internal/store"
	"github.com/magnetar/magnetar/internal/tracker"
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

// fireTrackerScrapes launches async tracker scrapes for newly persisted torrents.
func (c *Crawler) fireTrackerScrapes(ctx context.Context, torrents []*store.Torrent) {
	for _, t := range torrents {
		go func(torrent *store.Torrent) {
			var hash [20]byte
			copy(hash[:], torrent.InfoHash)

			result := c.trackerScraper.Scrape(ctx, hash)
			if result.Seeders == 0 && result.Leechers == 0 {
				return
			}

			c.updateTrackerCounts(ctx, torrent, result)
		}(t)
	}
}

func (c *Crawler) updateTrackerCounts(ctx context.Context, torrent *store.Torrent, result tracker.ScrapeResult) {
	existing, err := c.store.GetTorrent(ctx, torrent.InfoHash)
	if err != nil {
		return
	}

	updated := false
	if result.Seeders > existing.Seeders {
		existing.Seeders = result.Seeders
		updated = true
	}
	if result.Leechers > existing.Leechers {
		existing.Leechers = result.Leechers
		updated = true
	}

	if updated {
		existing.UpdatedAt = time.Now().Unix()
		if err := c.store.UpsertTorrent(ctx, existing); err != nil {
			c.logger.Error("failed to update tracker counts", "error", err)
		}
	}
}

// runPersistSources receives scrape results and updates seeders/leechers.
func (c *Crawler) runPersistSources(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case scrapes := <-c.persistSources.Out():
			for _, s := range scrapes {
				seeders := int(s.bfsd.ApproximatedSize())
				leechers := int(s.bfpe.ApproximatedSize())

				// Look up existing torrent and update S/L
				existing, err := c.store.GetTorrent(ctx, s.infoHash[:])
				if err != nil {
					continue
				}

				existing.Seeders = seeders
				existing.Leechers = leechers
				existing.UpdatedAt = time.Now().Unix()

				if upsertErr := c.store.UpsertTorrent(ctx, existing); upsertErr != nil {
					c.logger.Error("failed to update torrent source", "error", upsertErr)
				}
			}
			c.logger.Debug("persisted torrent sources", "count", len(scrapes))
		}
	}
}
