package gateway

import (
	"sync"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

type IndexEntry struct {
	Locations [3]proto.Location
	Length    uint32
	Seq       uint64
	// Adding extra element
	GatewayID uint8
}

type Index struct {
	mu      sync.RWMutex
	entries map[string]IndexEntry
}

func newIndex() *Index {
	return &Index{entries: make(map[string]IndexEntry)}
}

func (ix *Index) Get(key string) (IndexEntry, bool) {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	e, ok := ix.entries[key]
	return e, ok
}

func (ix *Index) Set(key string, e IndexEntry) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.entries[key] = e
}

func (ix *Index) SetIfNewer(key string, e IndexEntry) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	existing, ok := ix.entries[key]
	//if !ok || e.Seq > existing.Seq {
	//	ix.entries[key] = e
	//}
	if !ok || isNewerEntry(e, existing) {
		ix.entries[key] = e
	}
}

func isNewerEntry(candidate, existing IndexEntry) bool {
	if candidate.Seq != existing.Seq {
		return candidate.Seq > existing.Seq
	}
	return candidate.GatewayID > existing.GatewayID
}
