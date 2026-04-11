package proto

type Location struct {
   SSD uint8
   LBA uint64
}

type Phase1Msg struct {
   GatewayID uint8
   Seq uint64
   KeyHash uint64
   Key string
}

type Phase2Msg struct {
   GatewayID uint8
   Seq uint64
   Locations [3]Location
   Length uint32
}

type AppendReq struct {
   Key string
   Value []byte
}

type AppendResp struct {
   LBA uint64
}

type ReadReq struct {
   LBA uint64
}

type ReadResp struct {
   Value []byte
}


