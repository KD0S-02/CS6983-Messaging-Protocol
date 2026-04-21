package transport

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

type SSDClient struct {
	addr string
	mu   sync.Mutex
	conn net.Conn
}

func NewSSDClient(addr string) *SSDClient {
	return &SSDClient{addr: addr}
}

// error type hold
type applicationError struct {
	err error
}

func (e applicationError) Error() string { return e.err.Error() }
func (e applicationError) Unwrap() error { return e.err }

func nonRetryable(err error) error {
	return applicationError{err: err}
}

func (c *SSDClient) dial() error {
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return fmt.Errorf("ssd dial %s: %w", c.addr, err)
	}
	c.conn = conn
	return nil
}

// do executes fn with a live connection, retrying on transient errors
func (c *SSDClient) do(fn func(net.Conn) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if c.conn == nil {
			if err := c.dial(); err != nil {
				lastErr = err
				time.Sleep(retryDelay)
				continue
			}
		}
		if err := fn(c.conn); err != nil {
			var appErr applicationError
			if errors.As(err, &appErr) {
				return appErr.err
			}
			c.conn.Close()
			c.conn = nil
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		return nil
	}
	return fmt.Errorf("ssd %s: %w", c.addr, lastErr)
}

func (c *SSDClient) Append(key string, value []byte) (uint64, error) {
	var lba uint64
	err := c.do(func(conn net.Conn) error {
		if err := proto.WriteMsg(conn, proto.MsgAppendReq, proto.AppendReq{
			Key: key, Value: value,
		}); err != nil {
			return err
		}
		t, err := proto.ReadMsgType(conn)
		if err != nil {
			return err
		}
		if t != proto.MsgAppendResp {
			return fmt.Errorf("expected AppendResp got %d", t)
		}
		var resp proto.AppendResp
		if err := proto.ReadPayload(conn, &resp); err != nil {
			return err
		}
		//if resp.Err != "" {
		//	return errors.New(resp.Err)
		//}
		if resp.Err != "" {
			return nonRetryable(errors.New(resp.Err))
		}
		lba = resp.LBA
		return nil
	})
	return lba, err
}

func (c *SSDClient) Read(lba uint64) ([]byte, error) {
	var value []byte
	err := c.do(func(conn net.Conn) error {
		if err := proto.WriteMsg(conn, proto.MsgReadReq, proto.ReadReq{LBA: lba}); err != nil {
			return err
		}
		t, err := proto.ReadMsgType(conn)
		if err != nil {
			return err
		}
		if t != proto.MsgReadResp {
			return fmt.Errorf("expected ReadResp got %d", t)
		}
		var resp proto.ReadResp
		if err := proto.ReadPayload(conn, &resp); err != nil {
			return err
		}
		if resp.Err != "" {
			return nonRetryable(errors.New(resp.Err))
		}
		value = resp.Value
		return nil
	})
	return value, err
}
