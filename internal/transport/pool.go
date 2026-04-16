package transport

import (
	"net"
	"sync"
)

type connPool struct {
	addr    string
	mu      sync.Mutex
	free    []net.Conn
	maxSize int
}

func newConnPool(addr string, size int) *connPool {
	return &connPool{addr: addr, maxSize: size}
}

func (p *connPool) get() (net.Conn, error) {
	p.mu.Lock()
	if len(p.free) > 0 {
		conn := p.free[len(p.free)-1]
		p.free = p.free[:len(p.free)-1]
		p.mu.Unlock()
		return conn, nil
	}
	p.mu.Unlock()
	return net.Dial("tcp", p.addr)
}

func (p *connPool) put(conn net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.free) >= p.maxSize {
		conn.Close()
		return
	}
	p.free = append(p.free, conn)
}

func (p *connPool) discard(conn net.Conn) {
	conn.Close()
}
