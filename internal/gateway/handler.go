package gateway

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

var ErrNotFound = errors.New("key not found")

func (g *Gateway) HandlePUT(key string, value []byte) error {
	seq := g.seq.Add(1)

	// Phase 1 to all peers — fire and forget
	p1 := proto.Phase1Msg{GatewayID: g.ID, Seq: seq, Key: key}
	for _, p := range g.peers {
		p := p
		go func() {
			if err := p.SendPhase1(p1); err != nil {
				fmt.Printf("warn: gw-%d phase1 to %v failed: %v\n", g.ID, p, err)
			}
		}()
	}

	// Parallel APPEND to all 3 SSDs
	type ssdResult struct {
		idx uint8
		lba uint64
		err error
	}
	ch := make(chan ssdResult, 3)
	for i, ssd := range g.ssds {
		i, ssd := uint8(i), ssd
		go func() {
			lba, err := ssd.Append(key, value)
			ch <- ssdResult{i, lba, err}
		}()
	}

	var locs [3]proto.Location
	var mu sync.Mutex
	var firstErr error
	for range g.ssds {
		r := <-ch
		if r.err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = fmt.Errorf("ssd-%d: %w", r.idx, r.err)
			}
			mu.Unlock()
			continue
		}
		locs[r.idx] = proto.Location{SSD: r.idx, LBA: r.lba}
	}
	if firstErr != nil {
		// clear the phase1 without installing metadata
		go g.broadcastPhase2(key, seq, locs, 0, firstErr)
		return firstErr
	}

	// Update own index
	g.idx.Set(key, IndexEntry{
		Locations: locs,
		Length:    uint32(len(value)),
		Seq:       seq,
		// adding gateway
		GatewayID: g.ID,
	})

	// Phase 2 async — PUT is complete from the client's perspective
	go g.broadcastPhase2(key, seq, locs, uint32(len(value)), nil)

	return nil
}

func (g *Gateway) broadcastPhase2(key string, seq uint64, locs [3]proto.Location, length uint32, writeErr error) {
	errText := ""
	if writeErr != nil {
		errText = writeErr.Error()
	}

	msg := proto.Phase2Msg{
		GatewayID: g.ID,
		Seq:       seq,
		Key:       key,
		Locations: locs,
		Length:    length,
		Err:       errText,
	}

	for _, p := range g.peers {
		p := p
		go func() {
			if err := p.SendPhase2(msg); err != nil {
				fmt.Printf("warn: gw-%d phase2 failed: %v\n", g.ID, err)
			}
		}()
	}
}

func (g *Gateway) HandleGET(key string) ([]byte, error) {
	ow, pending := g.ow.Lookup(key)
	if pending {
		if err := g.ow.WaitFor(ow.GatewayID, ow.Seq, 5*time.Second); err != nil {
			return nil, fmt.Errorf("GET blocked: %w", err)
		}
	}

	entry, ok := g.idx.Get(key)
	if !ok {
		return nil, ErrNotFound
	}

	//loc := entry.Locations[0]
	//return g.ssds[loc.SSD].Read(loc.LBA)
	var lastErr error
	for _, loc := range entry.Locations {
		if int(loc.SSD) >= len(g.ssds) || g.ssds[loc.SSD] == nil {
			continue
		}

		value, err := g.ssds[loc.SSD].Read(loc.LBA)
		if err == nil {
			return value, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNotFound
}

func (g *Gateway) OnPhase1(msg proto.Phase1Msg) {
	g.ow.Add(OutstandingWrite{
		GatewayID: msg.GatewayID,
		Seq:       msg.Seq,
		Key:       msg.Key,
	})
}

func (g *Gateway) OnPhase2(msg proto.Phase2Msg) {
	g.ow.Remove(msg.GatewayID, msg.Seq, msg.Key)

	if msg.Err != "" {
		return
	}
	g.idx.SetIfNewer(msg.Key, IndexEntry{
		Locations: msg.Locations,
		Length:    msg.Length,
		Seq:       msg.Seq,
		GatewayID: msg.GatewayID,
	})
}
