package transport

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

const maxRetries = 3
const retryDelay = 50 * time.Millisecond

type PeerClient struct {
	addr string
	mu   sync.Mutex
	conn net.Conn
}

func NewPeerClient(addr string) *PeerClient {
	return &PeerClient{addr: addr}
}

func (c *PeerClient) dial() error {
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("peer dial %s: %w", c.addr, err)
	}
	c.conn = conn
	return nil
}

func (c *PeerClient) send(t proto.MsgType, v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt < maxRetries; attempt++ {
		if c.conn == nil {
			if err := c.dial(); err != nil {
				time.Sleep(retryDelay)
				continue
			}
		}
		if err := proto.WriteMsg(c.conn, t, v); err != nil {
			c.conn.Close()
			c.conn = nil
			time.Sleep(retryDelay)
			continue
		}
		return nil
	}
	return fmt.Errorf("peer %s: failed after %d attempts", c.addr, maxRetries)
}

func (c *PeerClient) SendPhase1(msg proto.Phase1Msg) error {
	return c.send(proto.MsgPhase1, msg)
}

func (c *PeerClient) SendPhase2(msg proto.Phase2Msg) error {
	return c.send(proto.MsgPhase2, msg)
}
