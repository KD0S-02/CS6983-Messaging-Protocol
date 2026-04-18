package gateway

import (
	"testing"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
	ssdpkg "github.com/KD0S-02/cs6983-messaging-protocol/internal/ssd"
	"github.com/KD0S-02/cs6983-messaging-protocol/internal/transport"
)

func TestIndexSetIfNewer(t *testing.T) {
	t.Parallel()

	ix := newIndex()
	ix.Set("k", IndexEntry{Seq: 5, Length: 5})
	ix.SetIfNewer("k", IndexEntry{Seq: 4, Length: 4})

	got, ok := ix.Get("k")
	if !ok {
		t.Fatal("index lost key")
	}
	if got.Seq != 5 || got.Length != 5 {
		t.Fatalf("stale SetIfNewer overwrote entry: %#v", got)
	}

	ix.SetIfNewer("k", IndexEntry{Seq: 6, Length: 6})
	got, ok = ix.Get("k")
	if !ok {
		t.Fatal("index lost key after newer update")
	}
	if got.Seq != 6 || got.Length != 6 {
		t.Fatalf("newer SetIfNewer did not update entry: %#v", got)
	}
}

func TestOutstandingMultipleWaitersAreReleased(t *testing.T) {
	t.Parallel()

	ow := newOutstandingWrites()
	ow.Add(OutstandingWrite{GatewayID: 1, Seq: 2, Key: "k"})

	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			errCh <- ow.WaitFor(1, 2, time.Second)
		}()
	}

	time.Sleep(50 * time.Millisecond)
	ow.Remove(1, 2, "k")

	for i := 0; i < 2; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("WaitFor() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("waiter did not unblock")
		}
	}
}

func TestOutstandingTimeoutRemovesWaiter(t *testing.T) {
	t.Parallel()

	ow := newOutstandingWrites()
	err := ow.WaitFor(1, 2, 20*time.Millisecond)
	if err == nil {
		t.Fatal("WaitFor() unexpectedly succeeded")
	}

	ow.waitMu.Lock()
	defer ow.waitMu.Unlock()
	if len(ow.waiters) != 0 {
		t.Fatalf("timeout left waiters behind: %#v", ow.waiters)
	}
}

func TestRemoveDoesNotClearNewerOutstandingWriteForSameKey(t *testing.T) {
	t.Parallel()

	ow := newOutstandingWrites()
	ow.Add(OutstandingWrite{GatewayID: 1, Seq: 1, Key: "k"})
	ow.Add(OutstandingWrite{GatewayID: 2, Seq: 2, Key: "k"})

	ow.Remove(1, 1, "k")

	got, ok := ow.Lookup("k")
	if !ok {
		t.Fatal("Remove(old write) cleared a newer outstanding write for the same key")
	}
	if got.GatewayID != 2 || got.Seq != 2 {
		t.Fatalf("remaining outstanding write = %#v, want gw=2 seq=2", got)
	}
}

func TestPhase2BeforePhase1DoesNotLeaveKeyPending(t *testing.T) {
	t.Parallel()

	gw := New(1, [3]*transport.SSDClient{}, nil)
	msg1 := proto.Phase1Msg{GatewayID: 2, Seq: 7, Key: "k"}
	msg2 := proto.Phase2Msg{GatewayID: 2, Seq: 7, Key: "k"}

	gw.OnPhase2(msg2)
	gw.OnPhase1(msg1)

	if _, pending := gw.ow.Lookup("k"); pending {
		t.Fatal("late Phase1 left key pending even though matching Phase2 had already arrived")
	}
}

func TestOnPhase2StaleSeqDoesNotOverwriteNewerIndexEntry(t *testing.T) {
	t.Parallel()

	gw := New(1, [3]*transport.SSDClient{}, nil)
	newer := IndexEntry{Seq: 10, Length: 10, Locations: [3]proto.Location{{SSD: 0, LBA: 10}}}
	gw.idx.Set("k", newer)

	gw.OnPhase2(proto.Phase2Msg{
		GatewayID: 2,
		Seq:       9,
		Key:       "k",
		Length:    9,
		Locations: [3]proto.Location{{SSD: 0, LBA: 9}},
	})

	got, ok := gw.idx.Get("k")
	if !ok {
		t.Fatal("index lost key")
	}
	if got.Seq != newer.Seq || got.Length != newer.Length || got.Locations[0].LBA != newer.Locations[0].LBA {
		t.Fatalf("stale Phase2 overwrote newer index entry: %#v", got)
	}
}

func TestHandlePUTWritesAllReplicasAndIndexesLocations(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}
	clients := buildSSDClients(ssds)
	gw := New(1, clients, nil)

	key := "replicas"
	want := []byte("same value everywhere")
	if err := gw.HandlePUT(key, want); err != nil {
		t.Fatalf("HandlePUT() error = %v", err)
	}

	entry, ok := gw.idx.Get(key)
	if !ok {
		t.Fatal("HandlePUT() did not update local index")
	}
	if entry.Length != uint32(len(want)) {
		t.Fatalf("index length = %d, want %d", entry.Length, len(want))
	}

	for i, loc := range entry.Locations {
		if int(loc.SSD) != i {
			t.Fatalf("location %d has SSD=%d, want %d", i, loc.SSD, i)
		}
		got, err := clients[loc.SSD].Read(loc.LBA)
		if err != nil {
			t.Fatalf("Read(replica %d) error = %v", i, err)
		}
		if string(got) != string(want) {
			t.Fatalf("replica %d value = %q, want %q", i, got, want)
		}
	}
}

func TestHandlePUTFailureDoesNotIndexLocalKey(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{FailRate: 1.0}),
		startSSD(t, 2, ssdpkg.Config{}),
	}
	gw := New(1, buildSSDClients(ssds), nil)

	key := "failed-local-put"
	if err := gw.HandlePUT(key, []byte("value")); err == nil {
		t.Fatal("HandlePUT() unexpectedly succeeded")
	}
	if _, ok := gw.idx.Get(key); ok {
		t.Fatal("failed HandlePUT() should not index the key locally")
	}
}
