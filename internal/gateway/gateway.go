package gateway

import (
	"sync/atomic"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/transport"
)

type Gateway struct {
	ID    uint8
	seq   atomic.Uint64
	idx   *Index
	ow    *OutstandingWrites
	ssds  [3]*transport.SSDClient
	peers []*transport.PeerClient
}

func New(id uint8, ssds [3]*transport.SSDClient, peers []*transport.PeerClient) *Gateway {
	return &Gateway{
		ID:    id,
		idx:   newIndex(),
		ow:    newOutstandingWrites(),
		ssds:  ssds,
		peers: peers,
	}
}
