package responder

import (
	"crypto/rand"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/dht/ktable"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"golang.org/x/time/rate"
)

// New creates a new DHT responder with rate limiting and node discovery.
func New(nodeID protocol.ID, kTable ktable.Table, discoveredNodes chan<- ktable.Node) Responder {
	tokenSecret := make([]byte, 20)
	_, _ = rand.Read(tokenSecret)

	base := &responder{
		nodeID:                   nodeID,
		kTable:                   kTable,
		tokenSecret:              tokenSecret,
		sampleInfoHashesInterval: 10,
	}

	limited := responderLimiter{
		responder: base,
		limiter:   NewLimiter(rate.Limit(2000), 100, rate.Limit(10), 5, 10000, 5*time.Minute),
	}

	return responderNodeDiscovery{
		responder:       limited,
		discoveredNodes: discoveredNodes,
	}
}
