package server

import (
	"context"
	"net/netip"

	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/dht"
)

type queryLimiter struct {
	server       Server
	queryLimiter concurrency.KeyedLimiter
}

func (s queryLimiter) Start() error {
	return s.server.Start()
}

func (s queryLimiter) Stop() {
	s.server.Stop()
}

func (s queryLimiter) Query(
	ctx context.Context,
	addr netip.AddrPort,
	q string,
	args dht.MsgArgs,
) (r dht.RecvMsg, err error) {
	if limitErr := s.queryLimiter.Wait(ctx, addr.Addr().String()); limitErr != nil {
		return r, limitErr
	}

	return s.server.Query(ctx, addr, q, args)
}
