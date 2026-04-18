package proto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadMultipleMessagesSameStream(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	first := Phase1Msg{GatewayID: 1, Seq: 10, Key: "first"}
	second := ReadReq{LBA: 123}

	if err := WriteMsg(&buf, MsgPhase1, first); err != nil {
		t.Fatalf("WriteMsg(first) error = %v", err)
	}
	if err := WriteMsg(&buf, MsgReadReq, second); err != nil {
		t.Fatalf("WriteMsg(second) error = %v", err)
	}

	typ, err := ReadMsgType(&buf)
	if err != nil {
		t.Fatalf("ReadMsgType(first) error = %v", err)
	}
	if typ != MsgPhase1 {
		t.Fatalf("first type = %v, want %v", typ, MsgPhase1)
	}
	var gotFirst Phase1Msg
	if err := ReadPayload(&buf, &gotFirst); err != nil {
		t.Fatalf("ReadPayload(first) error = %v", err)
	}
	if gotFirst != first {
		t.Fatalf("first payload = %#v, want %#v", gotFirst, first)
	}

	typ, err = ReadMsgType(&buf)
	if err != nil {
		t.Fatalf("ReadMsgType(second) error = %v", err)
	}
	if typ != MsgReadReq {
		t.Fatalf("second type = %v, want %v", typ, MsgReadReq)
	}
	var gotSecond ReadReq
	if err := ReadPayload(&buf, &gotSecond); err != nil {
		t.Fatalf("ReadPayload(second) error = %v", err)
	}
	if gotSecond != second {
		t.Fatalf("second payload = %#v, want %#v", gotSecond, second)
	}
}

func TestReadPayloadRejectsInvalidGobPayload(t *testing.T) {
	t.Parallel()

	var frame bytes.Buffer
	badPayload := []byte("this is not a gob payload")
	if err := frame.WriteByte(byte(MsgPhase1)); err != nil {
		t.Fatalf("WriteByte() error = %v", err)
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(badPayload)))
	frame.Write(lenBuf[:])
	frame.Write(badPayload)

	typ, err := ReadMsgType(&frame)
	if err != nil {
		t.Fatalf("ReadMsgType() error = %v", err)
	}
	if typ != MsgPhase1 {
		t.Fatalf("type = %v, want %v", typ, MsgPhase1)
	}

	var msg Phase1Msg
	if err := ReadPayload(&frame, &msg); err == nil {
		t.Fatal("ReadPayload() unexpectedly accepted invalid gob payload")
	}
}

func TestReadMsgTypeEOF(t *testing.T) {
	t.Parallel()

	_, err := ReadMsgType(bytes.NewReader(nil))
	if !errors.Is(err, io.EOF) {
		t.Fatalf("ReadMsgType() error = %v, want EOF", err)
	}
}

func TestWriteMsgPropagatesGobEncodeError(t *testing.T) {
	t.Parallel()

	err := WriteMsg(&bytes.Buffer{}, MsgPhase1, make(chan int))
	if err == nil {
		t.Fatal("WriteMsg() unexpectedly encoded an unsupported value")
	}
	if !strings.Contains(err.Error(), "gob encode") {
		t.Fatalf("WriteMsg() error = %q, want gob encode context", err.Error())
	}
}
