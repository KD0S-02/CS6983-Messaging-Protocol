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
	// adding element
	completed map[waitKey]struct{}
}

func newOutstandingWrites() *OutstandingWrites {
	return &OutstandingWrites{
		writes:  make(map[string]OutstandingWrite),
		waiters: make(map[waitKey][]chan struct{}),
		// completion
		completed: make(map[waitKey]struct{}),
	}
}

func (o *OutstandingWrites) Add(ow OutstandingWrite) {
	wk := waitKey{gwID: ow.GatewayID, seq: ow.Seq}

	o.waitMu.Lock()
	if _, done := o.completed[wk]; done {
		o.waitMu.Unlock()
		return
	}

	o.mu.Lock()
	//defer o.mu.Unlock()
	o.writes[ow.Key] = ow
	o.mu.Unlock()
	o.waitMu.Unlock()
}

func (o *OutstandingWrites) Lookup(key string) (OutstandingWrite, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	ow, ok := o.writes[key]
	return ow, ok
}

func (o *OutstandingWrites) Remove(gwID uint8, seq uint64, key string) {
	wk := waitKey{gwID: gwID, seq: seq}
	//o.mu.Lock()
	//delete(o.writes, key)
	//o.mu.Unlock()

	//wk := waitKey{gwID, seq}
	o.waitMu.Lock()
	o.completed[wk] = struct{}{}
	waiters := o.waiters[wk]
	delete(o.waiters, wk)
	o.mu.Lock()
	if current, ok := o.writes[key]; ok && current.GatewayID == gwID && current.Seq == seq {
		delete(o.writes, key)
	}
	o.mu.Unlock()

	o.waitMu.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
}

func (o *OutstandingWrites) WaitFor(gwID uint8, seq uint64, timeout time.Duration) error {
	wk := waitKey{gwID: gwID, seq: seq}
	ch := make(chan struct{})
	//wk := waitKey{gwID, seq}

	o.waitMu.Lock()
	if _, done := o.completed[wk]; done {
		o.waitMu.Unlock()
		return nil
	}

	o.waiters[wk] = append(o.waiters[wk], ch)
	o.waitMu.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		return nil
	//case <-time.After(timeout):
	//o.waitMu.Lock()
	//old := o.waiters[wk]
	//var kept []chan struct{}
	//for _, c := range old {
	//	if c != ch {
	//		kept = append(kept, c)
	//	}
	//}
	//if len(kept) == 0 {
	//	delete(o.waiters, wk)
	//} else {
	//	o.waiters[wk] = kept
	//}
	//o.waitMu.Unlock()
	//return fmt.Errorf("timeout waiting for phase2 gwID=%d seq=%d", gwID, seq)
	//}
	case <-timer.C:
		o.waitMu.Lock()
		if _, done := o.completed[wk]; done {
			o.waitMu.Unlock()
			return nil
		}

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
