package crawler

import (
	"context"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

// runInfoHashTriage receives discovered hashes, checks them against the Store,
// and routes them to the appropriate pipeline stage:
//   - Not in DB → getPeers (to fetch metadata)
//   - In DB with stale S/L → scrape (to refresh seeders/leechers)
//   - In DB and fresh → discard
func (c *Crawler) runInfoHashTriage(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case reqs := <-c.infoHashTriage.Out():
			allHashes := make([][]byte, 0, len(reqs))
			reqMap := make(map[protocol.ID]nodeHasPeersForHash, len(reqs))
			for _, r := range reqs {
				if _, ok := reqMap[r.infoHash]; ok {
					continue
				}
				allHashes = append(allHashes, r.infoHash[:])
				reqMap[r.infoHash] = r
			}

			if len(allHashes) == 0 {
				continue
			}

			existing, lookupErr := c.store.BulkLookup(ctx, allHashes)
			if lookupErr != nil {
				c.logger.Error("failed to lookup infohashes", "error", lookupErr)
				continue
			}

			// Build set of existing hashes
			existingMap := make(map[protocol.ID]existingInfo, len(existing))
			for _, t := range existing {
				var id protocol.ID
				copy(id[:], t.InfoHash)
				existingMap[id] = existingInfo{
					hasFiles:  len(t.Files) > 0,
					seeders:   t.Seeders,
					leechers:  t.Leechers,
					updatedAt: t.UpdatedAt,
				}
			}

			// Collect hashes not in the torrents table to check the reject list
			var unknownHashes [][]byte
			for hash := range reqMap {
				if _, found := existingMap[hash]; !found {
					unknownHashes = append(unknownHashes, hash[:])
				}
			}

			// Check rejected hashes — skip metadata fetch for known junk
			rejectedMap := make(map[protocol.ID]bool)
			if len(unknownHashes) > 0 {
				rejected, rejectErr := c.store.AreRejected(ctx, unknownHashes)
				if rejectErr != nil {
					c.logger.Error("failed to check rejected hashes", "error", rejectErr)
				} else {
					for h := range rejected {
						var id protocol.ID
						copy(id[:], h[:])
						rejectedMap[id] = true
					}
				}
			}

			for hash, req := range reqMap {
				info, found := existingMap[hash]
				switch {
				case rejectedMap[hash]:
					// Known junk — skip entirely
					continue
				case !found:
					// New hash — fetch metadata
					select {
					case <-ctx.Done():
						return
					case c.getPeers.In() <- req:
						continue
					}
				case !info.hasFiles:
					// Exists but missing file info — re-fetch
					select {
					case <-ctx.Done():
						return
					case c.getPeers.In() <- req:
						continue
					}
				case info.seeders == 0 && info.leechers == 0 ||
					time.Unix(info.updatedAt, 0).Before(time.Now().Add(-c.rescrapeThreshold)):
					// Stale S/L counts — rescrape
					select {
					case <-ctx.Done():
						return
					case c.scrape.In() <- req:
						continue
					}
				}
				// Otherwise: fresh and complete, discard
			}
		}
	}
}

type existingInfo struct {
	hasFiles  bool
	seeders   int
	leechers  int
	updatedAt int64
}
