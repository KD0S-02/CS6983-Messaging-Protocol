package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
)

type MsgType uint8

const (
	MsgPhase1     MsgType = 1
	MsgPhase2     MsgType = 2
	MsgAppendReq  MsgType = 3
	MsgAppendResp MsgType = 4
	MsgReadReq    MsgType = 5
	MsgReadResp   MsgType = 6
)

func WriteMsg(w io.Writer, t MsgType, v any) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return fmt.Errorf("gob encode type=%d: %w", t, err)
	}

	header := [5]byte{}
	header[0] = byte(t)
	binary.BigEndian.PutUint32(header[1:], uint32(buf.Len()))

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func ReadMsgType(r io.Reader) (MsgType, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return MsgType(b[0]), err
}

func ReadPayload(r io.Reader, v any) error {
	var lenBuf [4]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return fmt.Errorf("read length: %w", err)
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return fmt.Errorf("read payload: %w", err)
	}
	return gob.NewDecoder(bytes.NewReader(payload)).Decode(v)
}
