package metainforequester

import (
	"context"
	"net/netip"
	"time"

	"github.com/magnetar/magnetar/internal/crawler/concurrency"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

type requestLimiter struct {
	requester Requester
	limiter   concurrency.KeyedLimiter
}

func (r requestLimiter) Request(ctx context.Context, infoHash protocol.ID, node netip.AddrPort) (Response, error) {
	// Fail-fast: don't block more than 500ms waiting for the rate limiter.
	// This lets the fan-out try remaining peers instead of blocking a semaphore slot.
	limitCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if limitErr := r.limiter.Wait(limitCtx, node.Addr().String()); limitErr != nil {
		return Response{}, limitErr
	}
	return r.requester.Request(ctx, infoHash, node)
}
