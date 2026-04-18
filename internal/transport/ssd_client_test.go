package transport

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

func TestSSDClientDoesNotRetryApplicationErrors(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	var requests atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				for {
					msgType, err := proto.ReadMsgType(c)
					if err != nil {
						return
					}
					switch msgType {
					case proto.MsgAppendReq:
						var req proto.AppendReq
						if err := proto.ReadPayload(c, &req); err != nil {
							return
						}
						requests.Add(1)
						if err := proto.WriteMsg(c, proto.MsgAppendResp, proto.AppendResp{Err: "boom"}); err != nil {
							return
						}
					default:
						return
					}
				}
			}(conn)
		}
	}()

	client := NewSSDClient(ln.Addr().String())
	if _, err := client.Append("k", []byte("v")); err == nil {
		t.Fatal("Append() unexpectedly succeeded")
	}

	time.Sleep(100 * time.Millisecond)
	if got := requests.Load(); got != 1 {
		t.Fatalf("Append() sent %d requests for one application error, want 1", got)
	}

	ln.Close()
	<-done
}
