package server

import (
	"log/slog"
	"net/netip"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/dht"
	"github.com/magnetar/magnetar/internal/crawler/dht/responder"
	"golang.org/x/time/rate"
)

// New creates a new DHT server.
func New(cfg Config, nodeResp responder.Responder, logger *slog.Logger) Server {
	localAddr := netip.AddrPortFrom(netip.IPv4Unspecified(), cfg.Port)
	s := &server{
		stopped:          make(chan struct{}),
		localAddr:        localAddr,
		socket:           NewSocket(),
		queryTimeout:     cfg.QueryTimeout,
		queries:          make(map[string]chan dht.RecvMsg),
		responder:        nodeResp,
		responderTimeout: 5 * time.Second,
		idIssuer:         &variantIDIssuer{},
		logger:           logger,
	}
	// Apply rate limiter
	return queryLimiter{
		server:       s,
		queryLimiter: concurrency.NewKeyedLimiter(rate.Limit(500), 50, 10000, 5*time.Minute),
	}
}
