package ssd

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"

	"github.com/KD0S-02/cs6983-messaging-protocol/internal/proto"
)

type Server struct {
	ssd  *SSD
	addr string
}

func NewServer(ssd *SSD, addr string) *Server {
	return &Server{ssd: ssd, addr: addr}
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("ssd-%d listen %s: %w", s.ssd.ID, s.addr, err)
	}
	log.Printf("ssd-%d listening on %s datadir=%s", s.ssd.ID, s.addr, s.ssd.dataDir)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReaderSize(conn, 32*1024)

	for {
		t, err := proto.ReadMsgType(r)
		if err != nil {
			return // EOF or connection reset — normal
		}

		switch t {
		case proto.MsgAppendReq:
			var req proto.AppendReq
			if err := proto.ReadPayload(r, &req); err != nil {
				log.Printf("ssd-%d: decode AppendReq: %v", s.ssd.ID, err)
				return
			}
			lba, err := s.ssd.Append(req.Key, req.Value)
			resp := proto.AppendResp{LBA: lba}
			if err != nil {
				resp.Err = err.Error()
				log.Printf("ssd-%d: append error: %v", s.ssd.ID, err)
			}
			if err := proto.WriteMsg(conn, proto.MsgAppendResp, resp); err != nil {
				return
			}

		case proto.MsgReadReq:
			var req proto.ReadReq
			if err := proto.ReadPayload(r, &req); err != nil {
				log.Printf("ssd-%d: decode ReadReq: %v", s.ssd.ID, err)
				return
			}
			val, err := s.ssd.Read(req.LBA)
			resp := proto.ReadResp{Value: val}
			if err != nil {
				//if !errors.Is(err, proto.ErrNotFound) {
				if !errors.Is(err, ErrNotFound) {
					log.Printf("ssd-%d: read lba=%d: %v", s.ssd.ID, req.LBA, err)
				}
				resp.Err = err.Error()
			}
			if err := proto.WriteMsg(conn, proto.MsgReadResp, resp); err != nil {
				return
			}

		default:
			log.Printf("ssd-%d: unknown msg type %d, dropping connection", s.ssd.ID, t)
			return
		}
	}
}
