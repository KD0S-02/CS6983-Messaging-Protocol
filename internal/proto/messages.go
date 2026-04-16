package proto

type Location struct {
	SSD uint8
	LBA uint64
}

type IndexEntry struct {
	Locations [3]Location
	Length    uint32
	Seq       uint64
}

type Phase1Msg struct {
	GatewayID uint8
	Seq       uint64
	Key       string
}

type Phase2Msg struct {
	GatewayID uint8
	Seq       uint64
	Key       string
	Locations [3]Location
	Length    uint32
}

type AppendReq struct {
	Key   string
	Value []byte
}

type AppendResp struct {
	LBA uint64
	Err string
}

type ReadReq struct {
	LBA uint64
}

type ReadResp struct {
	Value []byte
	Err   string
}
