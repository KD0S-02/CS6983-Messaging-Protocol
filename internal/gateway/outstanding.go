package gateway

import (
	"fmt"
	"sync"
	"time"
)

type waitKey struct {
	gwID uint8
	seq  uint64
}

type OutstandingWrite struct {
	GatewayID uint8
	Seq       uint64
	Key       string
}

type OutstandingWrites struct {
	mu     sync.Mutex
	writes map[string]OutstandingWrite

	waitMu  sync.Mutex
	waiters map[waitKey][]chan struct{}
}

func newOutstandingWrites() *OutstandingWrites {
	return &OutstandingWrites{
		writes:  make(map[string]OutstandingWrite),
		waiters: make(map[waitKey][]chan struct{}),
	}
}

func (o *OutstandingWrites) Add(ow OutstandingWrite) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writes[ow.Key] = ow
}

func (o *OutstandingWrites) Lookup(key string) (OutstandingWrite, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	ow, ok := o.writes[key]
	return ow, ok
}

func (o *OutstandingWrites) Remove(gwID uint8, seq uint64, key string) {
	o.mu.Lock()
	delete(o.writes, key)
	o.mu.Unlock()

	wk := waitKey{gwID, seq}
	o.waitMu.Lock()
	waiters := o.waiters[wk]
	delete(o.waiters, wk)
	o.waitMu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
}

func (o *OutstandingWrites) WaitFor(gwID uint8, seq uint64, timeout time.Duration) error {
	ch := make(chan struct{})
	wk := waitKey{gwID, seq}

	o.waitMu.Lock()
	o.waiters[wk] = append(o.waiters[wk], ch)
	o.waitMu.Unlock()

	select {
	case <-ch:
		return nil
	case <-time.After(timeout):

		o.waitMu.Lock()
		old := o.waiters[wk]
		var kept []chan struct{}
		for _, c := range old {
			if c != ch {
				kept = append(kept, c)
			}
		}
		if len(kept) == 0 {
			delete(o.waiters, wk)
		} else {
			o.waiters[wk] = kept
		}
		o.waitMu.Unlock()
		return fmt.Errorf("timeout waiting for phase2 gwID=%d seq=%d", gwID, seq)
	}
}
