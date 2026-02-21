package dht

import (
	"github.com/anacrolix/torrent/bencode"
)

// Msg represents KRPC messages that DHT nodes exchange.
// Three types: QUERY (y="q"), RESPONSE (y="r"), ERROR (y="e").
type Msg struct {
	Q string   `bencode:"q,omitempty"`
	A *MsgArgs `bencode:"a,omitempty"`
	T string   `bencode:"t"`
	Y string   `bencode:"y"`
	R *Return  `bencode:"r,omitempty"`
	E *Error   `bencode:"e,omitempty"`
	// BEP 42 external IP notification
	IP NodeAddr `bencode:"ip,omitempty"`
	// BEP 43: sender does not respond to queries
	ReadOnly bool `bencode:"ro,omitempty"`
	// libtorrent client ID extension
	ClientID string `bencode:"v,omitempty"`
}

const (
	YQuery    = "q"
	YResponse = "r"
	YError    = "e"
)

type MsgArgs struct {
	ID       ID `bencode:"id"`
	InfoHash ID `bencode:"info_hash,omitempty"`
	Target   ID `bencode:"target,omitempty"`
	Token       string `bencode:"token,omitempty"`
	Port        *int   `bencode:"port,omitempty"`
	ImpliedPort bool   `bencode:"implied_port,omitempty"`
	Want        []Want `bencode:"want,omitempty"`
	NoSeed      int    `bencode:"noseed,omitempty"`
	Scrape      int    `bencode:"scrape,omitempty"`

	// BEP 44
	V    interface{} `bencode:"v,omitempty"`
	Seq  *int64      `bencode:"seq,omitempty"`
	Cas  int64       `bencode:"cas,omitempty"`
	K    [32]byte    `bencode:"k,omitempty"`
	Salt []byte      `bencode:"salt,omitempty"`
	Sig  [64]byte    `bencode:"sig,omitempty"`
}

type Want string

const (
	WantNodes  Want = "n4"
	WantNodes6 Want = "n6"
)

// BEP 51 (DHT Infohash Indexing)
type Bep51Return struct {
	Interval *int64             `bencode:"interval,omitempty"`
	Num      *int64             `bencode:"num,omitempty"`
	Samples  *CompactInfohashes `bencode:"samples,omitempty"`
}

type Bep44Return struct {
	V   bencode.Bytes `bencode:"v,omitempty"`
	K   [32]byte      `bencode:"k,omitempty"`
	Sig [64]byte      `bencode:"sig,omitempty"`
	Seq *int64        `bencode:"seq,omitempty"`
}

type Return struct {
	ID     ID                  `bencode:"id"`
	Nodes  CompactIPv4NodeInfo `bencode:"nodes,omitempty"`
	Nodes6 CompactIPv6NodeInfo `bencode:"nodes6,omitempty"`
	Token  *string             `bencode:"token,omitempty"`
	Values []NodeAddr          `bencode:"values,omitempty"`

	// BEP 33 (scrapes)
	BFsd *ScrapeBloomFilter `bencode:"BFsd,omitempty"`
	BFpe *ScrapeBloomFilter `bencode:"BFpe,omitempty"`

	Bep51Return
	Bep44Return
}

type CompactInfohashes []ID

func (CompactInfohashes) ElemSize() int { return 20 }

func (me CompactInfohashes) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(me)
}

func (me CompactInfohashes) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactInfohashes) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactInfohashes) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}

type CompactIPv6NodeInfo []NodeInfo

func (CompactIPv6NodeInfo) ElemSize() int { return 38 }

func (me CompactIPv6NodeInfo) MarshalBinary() ([]byte, error) {
	return marshalBinarySlice(me)
}

func (me CompactIPv6NodeInfo) MarshalBencode() ([]byte, error) {
	return bencodeBytesResult(me.MarshalBinary())
}

func (me *CompactIPv6NodeInfo) UnmarshalBinary(b []byte) error {
	return unmarshalBinarySlice(me, b)
}

func (me *CompactIPv6NodeInfo) UnmarshalBencode(b []byte) error {
	return unmarshalBencodedBinary(me, b)
}
