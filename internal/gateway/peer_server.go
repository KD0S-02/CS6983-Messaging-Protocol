package gateway

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

type PeerServer struct {
	gw   *Gateway
	addr string
}

func NewPeerServer(gw *Gateway, addr string) *PeerServer {
	return &PeerServer{gw: gw, addr: addr}
}

func (s *PeerServer) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("gateway-%d peer server listen %s: %w", s.gw.ID, s.addr, err)
	}
	log.Printf("gateway-%d peer listener on %s", &s.gw.ID, s.addr)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *PeerServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReaderSize(conn, 16*1024)

	for {
		t, err := proto.ReadMsgType(r)
		if err != nil {
			return
		}
		switch t {
		case proto.MsgPhase1:
			var msg proto.Phase1Msg
			if err := proto.ReadPayload(r, &msg); err != nil {
				log.Printf("gateway-%d: decode Phase1: %v", s.gw.ID, err)
				return
			}
			s.gw.OnPhase1(msg)
			log.Printf("gateway-%d: recv Phase1 from gw-%d seq=%d key=%s",
				s.gw.ID, msg.GatewayID, msg.Seq, msg.Key)

		case proto.MsgPhase2:
			var msg proto.Phase2Msg
			if err := proto.ReadPayload(r, &msg); err != nil {
				log.Printf("gateway-%d: decode Phase2: %v", s.gw.ID, err)
				return
			}
			s.gw.OnPhase2(msg)
			log.Printf("gateway-%d: recv Phase2 from gw-%d seq=%d key=%s lba=%d",
				s.gw.ID, msg.GatewayID, msg.Seq, msg.Key, msg.Locations[0].LBA)

		default:
			log.Printf("gateway-%d: unknown peer msg type %d", s.gw.ID, t)
			return
		}
	}
}
