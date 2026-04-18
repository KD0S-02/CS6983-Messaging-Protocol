package gateway

import (
	"bytes"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
	ssdpkg "github.com/KD0S-02/cs6983-messaging-protocol/internal/ssd"
	"github.com/KD0S-02/cs6983-messaging-protocol/internal/transport"
)

type startedSSD struct {
	addr   string
	client *transport.SSDClient
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func startSSD(t *testing.T, id uint8, cfg ssdpkg.Config) startedSSD {
	t.Helper()
	dir := t.TempDir()
	disk, err := ssdpkg.New(id, dir, cfg)
	if err != nil {
		t.Fatalf("ssd.New() error = %v", err)
	}
	addr := freeAddr(t)
	srv := ssdpkg.NewServer(disk, addr)
	go func() { _ = srv.ListenAndServe() }()
	waitForTCP(t, addr)
	return startedSSD{addr: addr, client: transport.NewSSDClient(addr)}
}

func startPeerServer(t *testing.T, gw *Gateway) string {
	t.Helper()
	addr := freeAddr(t)
	srv := NewPeerServer(gw, addr)
	go func() { _ = srv.ListenAndServe() }()
	waitForTCP(t, addr)
	return addr
}

func waitForTCP(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server on %s did not start listening", addr)
}

func waitEventually(t *testing.T, timeout time.Duration, cond func() bool, format string, args ...any) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf(format, args...)
}

func buildSSDClients(ssds [3]startedSSD) [3]*transport.SSDClient {
	return [3]*transport.SSDClient{ssds[0].client, ssds[1].client, ssds[2].client}
}

func appendToAll(t *testing.T, clients [3]*transport.SSDClient, key string, value []byte) [3]proto.Location {
	t.Helper()
	var locs [3]proto.Location
	for i, c := range clients {
		lba, err := c.Append(key, value)
		if err != nil {
			t.Fatalf("Append() to ssd-%d error = %v", i, err)
		}
		locs[i] = proto.Location{SSD: uint8(i), LBA: lba}
	}
	return locs
}

func TestHandleGETReturnsNotFound(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}
	gw := New(1, buildSSDClients(ssds), nil)

	_, err := gw.HandleGET("missing")
	if err != ErrNotFound {
		t.Fatalf("HandleGET() error = %v, want ErrNotFound", err)
	}
}

func TestPendingWriteBlocksThenUnblocksGET(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}
	gw := New(1, buildSSDClients(ssds), nil)

	key := "alpha"
	want := []byte("value-alpha")
	locs := appendToAll(t, buildSSDClients(ssds), key, want)

	gw.OnPhase1(proto.Phase1Msg{GatewayID: 9, Seq: 41, Key: key})

	resultCh := make(chan struct {
		value []byte
		err   error
	}, 1)
	go func() {
		value, err := gw.HandleGET(key)
		resultCh <- struct {
			value []byte
			err   error
		}{value: value, err: err}
	}()

	select {
	case <-resultCh:
		t.Fatal("HandleGET returned before Phase2 completed")
	case <-time.After(100 * time.Millisecond):
	}

	gw.OnPhase2(proto.Phase2Msg{
		GatewayID: 9,
		Seq:       41,
		Key:       key,
		Locations: locs,
		Length:    uint32(len(want)),
	})

	select {
	case res := <-resultCh:
		if res.err != nil {
			t.Fatalf("HandleGET() error = %v", res.err)
		}
		if !bytes.Equal(res.value, want) {
			t.Fatalf("HandleGET() = %q, want %q", res.value, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HandleGET did not unblock after Phase2")
	}
}

func TestEndToEndPutOnGateway1ReadableFromGateway2(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}

	gw1 := New(1, buildSSDClients(ssds), nil)
	gw2 := New(2, buildSSDClients(ssds), nil)

	gw1PeerAddr := startPeerServer(t, gw1)
	gw2PeerAddr := startPeerServer(t, gw2)

	gw1.peers = []*transport.PeerClient{transport.NewPeerClient(gw2PeerAddr)}
	gw2.peers = []*transport.PeerClient{transport.NewPeerClient(gw1PeerAddr)}

	key := "user:42"
	want := []byte("hello from gateway 1")
	if err := gw1.HandlePUT(key, want); err != nil {
		t.Fatalf("HandlePUT() error = %v", err)
	}

	waitEventually(t, 2*time.Second, func() bool {
		_, ok := gw2.idx.Get(key)
		return ok
	}, "gateway 2 never learned key %q", key)

	got, err := gw2.HandleGET(key)
	if err != nil {
		t.Fatalf("gw2.HandleGET() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("gw2.HandleGET() = %q, want %q", got, want)
	}
}

func TestConcurrentWritersSameKeyConvergeToLatestValue(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}

	gw1 := New(1, buildSSDClients(ssds), nil)
	gw2 := New(2, buildSSDClients(ssds), nil)

	gw1PeerAddr := startPeerServer(t, gw1)
	gw2PeerAddr := startPeerServer(t, gw2)

	gw1.peers = []*transport.PeerClient{transport.NewPeerClient(gw2PeerAddr)}
	gw2.peers = []*transport.PeerClient{transport.NewPeerClient(gw1PeerAddr)}

	key := "shared-key"
	v1 := []byte("value-from-gw1")
	v2 := []byte("value-from-gw2")

	if err := gw1.HandlePUT(key, v1); err != nil {
		t.Fatalf("gw1.HandlePUT() error = %v", err)
	}
	waitEventually(t, 2*time.Second, func() bool {
		got, err := gw2.HandleGET(key)
		return err == nil && bytes.Equal(got, v1)
	}, "gateway 2 never observed first write")

	if err := gw2.HandlePUT(key, v2); err != nil {
		t.Fatalf("gw2.HandlePUT() error = %v", err)
	}

	waitEventually(t, 2*time.Second, func() bool {
		got, err := gw1.HandleGET(key)
		return err == nil && bytes.Equal(got, v2)
	}, "gateway 1 did not converge to the later write from gateway 2")

	got2, err := gw2.HandleGET(key)
	if err != nil {
		t.Fatalf("gw2.HandleGET() error = %v", err)
	}
	if !bytes.Equal(got2, v2) {
		t.Fatalf("gw2.HandleGET() = %q, want %q", got2, v2)
	}
}

func TestFailedPUTClearsPeerPendingState(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{}),
		startSSD(t, 1, ssdpkg.Config{FailRate: 1.0}),
		startSSD(t, 2, ssdpkg.Config{}),
	}

	gw1 := New(1, buildSSDClients(ssds), nil)
	gw2 := New(2, buildSSDClients(ssds), nil)

	gw1PeerAddr := startPeerServer(t, gw1)
	gw2PeerAddr := startPeerServer(t, gw2)

	gw1.peers = []*transport.PeerClient{transport.NewPeerClient(gw2PeerAddr)}
	gw2.peers = []*transport.PeerClient{transport.NewPeerClient(gw1PeerAddr)}

	key := "will-fail"
	if err := gw1.HandlePUT(key, []byte("value")); err == nil {
		t.Fatal("gw1.HandlePUT() unexpectedly succeeded")
	}

	waitEventually(t, 2*time.Second, func() bool {
		_, pending := gw2.ow.Lookup(key)
		return pending
	}, "gateway 2 never observed pending write for %q", key)

	waitEventually(t, 500*time.Millisecond, func() bool {
		_, pending := gw2.ow.Lookup(key)
		return !pending
	}, "gateway 2 still thinks %q is pending after failed PUT", key)
}

func TestHandleGETFallsBackToHealthyReplica(t *testing.T) {
	ssds := [3]startedSSD{
		startSSD(t, 0, ssdpkg.Config{FailRate: 1.0}),
		startSSD(t, 1, ssdpkg.Config{}),
		startSSD(t, 2, ssdpkg.Config{}),
	}
	clients := buildSSDClients(ssds)
	gw := New(1, clients, nil)

	key := "replicated"
	want := []byte("durable-value")

	lba1, err := clients[1].Append(key, want)
	if err != nil {
		t.Fatalf("ssd1.Append() error = %v", err)
	}
	lba2, err := clients[2].Append(key, want)
	if err != nil {
		t.Fatalf("ssd2.Append() error = %v", err)
	}

	gw.idx.Set(key, IndexEntry{
		Locations: [3]proto.Location{{SSD: 0, LBA: 0}, {SSD: 1, LBA: lba1}, {SSD: 2, LBA: lba2}},
		Length:    uint32(len(want)),
		Seq:       1,
	})

	got, err := gw.HandleGET(key)
	if err != nil {
		t.Fatalf("HandleGET() error = %v; expected failover to healthy replica", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("HandleGET() = %q, want %q", got, want)
	}
}

func TestOutstandingWriteCompletionIsNotMissedBetweenLookupAndWait(t *testing.T) {
	ow := newOutstandingWrites()
	ow.Add(OutstandingWrite{GatewayID: 3, Seq: 99, Key: "k"})

	current, ok := ow.Lookup("k")
	if !ok {
		t.Fatal("Lookup() did not find outstanding write")
	}

	ow.Remove(current.GatewayID, current.Seq, current.Key)

	if err := ow.WaitFor(current.GatewayID, current.Seq, 50*time.Millisecond); err != nil {
		t.Fatalf("WaitFor() error = %v; expected already-completed write to return immediately", err)
	}
}

func TestHarnessSanity(t *testing.T) {
	addr := freeAddr(t)
	if addr == "" {
		t.Fatal("freeAddr() returned empty address")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		t.Fatalf("freeAddr() returned invalid address %q: %v", addr, err)
	}
	if got := fmt.Sprintf("%T", buildSSDClients([3]startedSSD{})); got == "" {
		t.Fatal("unexpected empty type formatting")
	}
}
