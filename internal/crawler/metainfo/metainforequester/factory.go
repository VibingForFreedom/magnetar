package metainforequester

import (
	"net"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
	"golang.org/x/time/rate"
)

// New creates a new metadata requester with rate limiting.
func New(cfg Config) Requester {
	clientID := protocol.RandomPeerID()
	base := &requester{
		clientID: clientID,
		timeout:  cfg.RequestTimeout,
		dialer: &net.Dialer{
			Timeout: cfg.RequestTimeout,
		},
	}
	return requestLimiter{
		requester: base,
		limiter:   concurrency.NewKeyedLimiter(rate.Limit(5), 2, cfg.KeyMutexSize, 5*time.Minute),
	}
}
