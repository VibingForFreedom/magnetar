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

			for _, item := range items {
				if _, ok := hashMap[item.infoHash]; ok {
					continue
				}
				hashMap[item.infoHash] = item

				t, ok := c.buildTorrent(item.infoHash, item.metaInfo, item.peerCount)
				if !ok {
					continue
				}
				torrents = append(torrents, t)
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
	var classifyFiles []classify.File
	var storeFiles []store.File

	for i, file := range info.Files {
		if i >= int(c.saveFilesThreshold) {
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
