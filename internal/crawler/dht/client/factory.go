package client

import (
	"github.com/magnetar/magnetar/internal/crawler/dht/server"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

// New creates a new DHT client from a server.
func New(nodeID protocol.ID, srv server.Server) Client {
	return serverAdapter{
		nodeID: nodeID,
		server: srv,
	}
}
