package proto

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		msgType MsgType
		value   any
		newPtr  func() any
	}{
		{
			name:    "phase1",
			msgType: MsgPhase1,
			value:   Phase1Msg{GatewayID: 2, Seq: 9, Key: "alpha"},
			newPtr:  func() any { return &Phase1Msg{} },
		},
		{
			name:    "phase2",
			msgType: MsgPhase2,
			value: Phase2Msg{
				GatewayID: 1,
				Seq:       7,
				Key:       "beta",
				Locations: [3]Location{{SSD: 0, LBA: 11}, {SSD: 1, LBA: 12}, {SSD: 2, LBA: 13}},
				Length:    5,
			},
			newPtr: func() any { return &Phase2Msg{} },
		},
		{
			name:    "append_req",
			msgType: MsgAppendReq,
			value:   AppendReq{Key: "k", Value: []byte("value")},
			newPtr:  func() any { return &AppendReq{} },
		},
		{
			name:    "append_resp",
			msgType: MsgAppendResp,
			value:   AppendResp{LBA: 44, Err: ""},
			newPtr:  func() any { return &AppendResp{} },
		},
		{
			name:    "read_req",
			msgType: MsgReadReq,
			value:   ReadReq{LBA: 91},
			newPtr:  func() any { return &ReadReq{} },
		},
		{
			name:    "read_resp",
			msgType: MsgReadResp,
			value:   ReadResp{Value: []byte("payload"), Err: ""},
			newPtr:  func() any { return &ReadResp{} },
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteMsg(&buf, tc.msgType, tc.value); err != nil {
				t.Fatalf("WriteMsg() error = %v", err)
			}

			gotType, err := ReadMsgType(&buf)
			if err != nil {
				t.Fatalf("ReadMsgType() error = %v", err)
			}
			if gotType != tc.msgType {
				t.Fatalf("ReadMsgType() = %v, want %v", gotType, tc.msgType)
			}

			gotPtr := tc.newPtr()
			if err := ReadPayload(&buf, gotPtr); err != nil {
				t.Fatalf("ReadPayload() error = %v", err)
			}

			got := reflect.ValueOf(gotPtr).Elem().Interface()
			if !reflect.DeepEqual(got, tc.value) {
				t.Fatalf("round trip mismatch\n got: %#v\nwant: %#v", got, tc.value)
			}
		})
	}
}

func TestReadPayloadTruncated(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := WriteMsg(&buf, MsgPhase1, Phase1Msg{GatewayID: 1, Seq: 2, Key: "k"}); err != nil {
		t.Fatalf("WriteMsg() error = %v", err)
	}
	data := buf.Bytes()
	if len(data) < 3 {
		t.Fatal("encoded message unexpectedly short")
	}

	truncated := bytes.NewReader(data[:len(data)-2])
	if _, err := ReadMsgType(truncated); err != nil {
		t.Fatalf("ReadMsgType() error = %v", err)
	}
	var msg Phase1Msg
	if err := ReadPayload(truncated, &msg); err == nil {
		t.Fatal("ReadPayload() unexpectedly succeeded on truncated payload")
	}
}
