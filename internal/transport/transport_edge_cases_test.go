package transport

import (
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
	ssdpkg "github.com/KD0S-02/cs6983-messaging-protocol/internal/ssd"
)

type recordingConn struct {
	closed atomic.Bool
}

func (c *recordingConn) Read(_ []byte) (int, error)         { return 0, io.EOF }
func (c *recordingConn) Write(p []byte) (int, error)        { return len(p), nil }
func (c *recordingConn) Close() error                       { c.closed.Store(true); return nil }
func (c *recordingConn) LocalAddr() net.Addr                { return dummyAddr("local") }
func (c *recordingConn) RemoteAddr() net.Addr               { return dummyAddr("remote") }
func (c *recordingConn) SetDeadline(_ time.Time) error      { return nil }
func (c *recordingConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *recordingConn) SetWriteDeadline(_ time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return string(d) }
func (d dummyAddr) String() string  { return string(d) }

func transportFreeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func transportWaitForTCP(t *testing.T, addr string) {
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
	t.Fatalf("server on %s did not start", addr)
}

func TestSSDClientAppendReadThroughServer(t *testing.T) {
	disk, err := ssdpkg.New(0, t.TempDir(), ssdpkg.Config{})
	if err != nil {
		t.Fatalf("ssd.New() error = %v", err)
	}
	addr := transportFreeAddr(t)
	srv := ssdpkg.NewServer(disk, addr)
	go func() { _ = srv.ListenAndServe() }()
	transportWaitForTCP(t, addr)

	client := NewSSDClient(addr)
	lba, err := client.Append("k", []byte("value"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	got, err := client.Read(lba)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if string(got) != "value" {
		t.Fatalf("Read() = %q, want %q", got, "value")
	}
}

func TestSSDClientReconnectsAfterDroppedConnection(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	var requests atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			msgType, err := proto.ReadMsgType(conn)
			if err != nil {
				conn.Close()
				return
			}
			if msgType != proto.MsgAppendReq {
				conn.Close()
				return
			}
			var req proto.AppendReq
			if err := proto.ReadPayload(conn, &req); err != nil {
				conn.Close()
				return
			}
			requests.Add(1)
			if i == 0 {
				conn.Close()
				continue
			}
			_ = proto.WriteMsg(conn, proto.MsgAppendResp, proto.AppendResp{LBA: 77})
			conn.Close()
		}
	}()

	client := NewSSDClient(ln.Addr().String())
	lba, err := client.Append("k", []byte("v"))
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if lba != 77 {
		t.Fatalf("Append() lba = %d, want 77", lba)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("server saw %d requests, want 2", got)
	}

	ln.Close()
	<-done
}

func TestPeerClientSendsPhaseMessages(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	got := make(chan proto.MsgType, 2)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		typ, err := proto.ReadMsgType(conn)
		if err != nil {
			return
		}
		var p1 proto.Phase1Msg
		if err := proto.ReadPayload(conn, &p1); err != nil {
			return
		}
		if p1.GatewayID == 1 && p1.Seq == 10 && p1.Key == "k" {
			got <- typ
		}

		typ, err = proto.ReadMsgType(conn)
		if err != nil {
			return
		}
		var p2 proto.Phase2Msg
		if err := proto.ReadPayload(conn, &p2); err != nil {
			return
		}
		if p2.GatewayID == 1 && p2.Seq == 10 && p2.Key == "k" && p2.Length == 3 {
			got <- typ
		}
	}()

	client := NewPeerClient(ln.Addr().String())
	if err := client.SendPhase1(proto.Phase1Msg{GatewayID: 1, Seq: 10, Key: "k"}); err != nil {
		t.Fatalf("SendPhase1() error = %v", err)
	}
	if err := client.SendPhase2(proto.Phase2Msg{GatewayID: 1, Seq: 10, Key: "k", Length: 3}); err != nil {
		t.Fatalf("SendPhase2() error = %v", err)
	}

	for _, want := range []proto.MsgType{proto.MsgPhase1, proto.MsgPhase2} {
		select {
		case typ := <-got:
			if typ != want {
				t.Fatalf("got msg type %v, want %v", typ, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %v", want)
		}
	}

	ln.Close()
	<-done
}

func TestConnPoolReusesFreeConnection(t *testing.T) {
	pool := newConnPool("unused", 1)
	conn := &recordingConn{}
	pool.put(conn)

	got, err := pool.get()
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	if got != conn {
		t.Fatalf("get() did not reuse the free connection")
	}
	if conn.closed.Load() {
		t.Fatal("reused connection was unexpectedly closed")
	}
}

func TestConnPoolClosesConnectionWhenFull(t *testing.T) {
	pool := newConnPool("unused", 1)
	first := &recordingConn{}
	second := &recordingConn{}

	pool.put(first)
	pool.put(second)

	if first.closed.Load() {
		t.Fatal("first connection should have stayed in the pool")
	}
	if !second.closed.Load() {
		t.Fatal("second connection should have been closed because pool was full")
	}
}

func TestConnPoolDiscardClosesConnection(t *testing.T) {
	pool := newConnPool("unused", 1)
	conn := &recordingConn{}
	pool.discard(conn)
	if !conn.closed.Load() {
		t.Fatal("discard() did not close the connection")
	}
}

func TestPeerClientReturnsErrorWhenDialFails(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	client := NewPeerClient(addr)
	err = client.SendPhase1(proto.Phase1Msg{GatewayID: 1, Seq: 1, Key: "k"})
	if err == nil {
		t.Fatal("SendPhase1() unexpectedly succeeded against closed listener")
	}
}
