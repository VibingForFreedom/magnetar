// Package metainforequester implements BEP 9 metadata fetching.
// Extracted from github.com/bitmagnet-io/bitmagnet (MIT License).
package metainforequester

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/netip"
	"time"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/peer_protocol"
	"github.com/magnetar/magnetar/internal/crawler/metainfo"
	"github.com/magnetar/magnetar/internal/crawler/protocol"
)

type Requester interface {
	Request(context.Context, protocol.ID, netip.AddrPort) (Response, error)
}

type requester struct {
	clientID protocol.ID
	timeout  time.Duration
	dialer   *net.Dialer
}

type ExtensionBit uint

const (
	ExtensionBitDht                         = 0
	ExtensionBitFast                        = 2
	ExtensionBitV2                          = 7
	ExtensionBitAzureusExtensionNegotiation1 = 16
	ExtensionBitAzureusExtensionNegotiation2 = 17
	ExtensionBitLtep                        = 20
	ExtensionBitLocationAwareProtocol       = 43
	ExtensionBitAzureusMessagingProtocol    = 63
)

type PeerExtensionBits [8]byte

func NewPeerExtensionBits(bits ...ExtensionBit) (ret PeerExtensionBits) {
	for _, b := range bits {
		ret = ret.WithBit(b, true)
	}
	return
}

func (pex PeerExtensionBits) WithBit(bit ExtensionBit, on bool) PeerExtensionBits {
	if on {
		pex[7-bit/8] |= 1 << (bit % 8)
	} else {
		pex[7-bit/8] &^= 1 << (bit % 8)
	}
	return pex
}

func (pex PeerExtensionBits) GetBit(bit ExtensionBit) bool {
	return pex[7-bit/8]&(1<<(bit%8)) != 0
}

type HandshakeInfo struct {
	PeerID            protocol.ID
	PeerExtensionBits PeerExtensionBits
}

type Response struct {
	HandshakeInfo
	Info metainfo.Info
}

func (r requester) Request(ctx context.Context, infoHash protocol.ID, addr netip.AddrPort) (Response, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	conn, connErr := r.connect(timeoutCtx, addr)
	if connErr != nil {
		return Response{}, connErr
	}
	defer func() { _ = conn.Close() }()

	hsInfo, btHandshakeErr := btHandshake(conn, infoHash, r.clientID)
	if btHandshakeErr != nil {
		return Response{}, btHandshakeErr
	}

	metadataSize, utMetadata, exHandshakeErr := exHandshake(conn)
	if exHandshakeErr != nil {
		return Response{}, exHandshakeErr
	}

	if requestAllPiecesErr := requestAllPieces(conn, metadataSize, utMetadata); requestAllPiecesErr != nil {
		return Response{}, requestAllPiecesErr
	}

	pieces, readAllPiecesErr := readAllPieces(conn, metadataSize)
	if readAllPiecesErr != nil {
		return Response{}, readAllPiecesErr
	}

	parsed, parseErr := metainfo.ParseMetaInfoBytes(infoHash, pieces)
	if parseErr != nil {
		return Response{}, parseErr
	}

	return Response{
		HandshakeInfo: hsInfo,
		Info:          parsed,
	}, nil
}

func (r requester) connect(ctx context.Context, addr netip.AddrPort) (conn *net.TCPConn, err error) {
	c, dialErr := r.dialer.DialContext(ctx, "tcp4", addr.String())
	if dialErr != nil {
		err = dialErr
		return
	}
	tcpConn := c.(*net.TCPConn)
	closeConn := func() { _ = tcpConn.Close() }

	if setLingerErr := tcpConn.SetLinger(0); setLingerErr != nil {
		err = setLingerErr
		closeConn()
		return
	}
	deadline, ok := ctx.Deadline()
	if ok {
		if setDeadlineErr := tcpConn.SetDeadline(deadline); setDeadlineErr != nil {
			err = setDeadlineErr
			closeConn()
			return
		}
	}
	return tcpConn, nil
}

var myExBits = NewPeerExtensionBits(ExtensionBitDht, ExtensionBitLtep)

func btHandshake(rw io.ReadWriter, infoHash protocol.ID, clientID protocol.ID) (HandshakeInfo, error) {
	handshakeBytes := make([]byte, 0, 68)
	handshakeBytes = append(handshakeBytes, peer_protocol.Protocol...)
	handshakeBytes = append(handshakeBytes, myExBits[:]...)
	handshakeBytes = append(handshakeBytes, infoHash[:]...)
	handshakeBytes = append(handshakeBytes, clientID[:]...)

	if n, hsErr := rw.Write(handshakeBytes); hsErr != nil {
		return HandshakeInfo{}, hsErr
	} else if n != 68 {
		panic("handshake bytes must have length 68")
	}

	handshakeResponse := make([]byte, 68)
	if n, hsErr := io.ReadFull(rw, handshakeResponse); hsErr != nil {
		return HandshakeInfo{},
			fmt.Errorf("failed to read all handshake bytes (%d): %w / %s", n, hsErr, infoHash.String())
	}

	if !bytes.HasPrefix(handshakeResponse, []byte(peer_protocol.Protocol)) {
		return HandshakeInfo{}, errors.New("invalid handshake response received")
	}

	var peerExBits PeerExtensionBits
	copy(peerExBits[:], handshakeResponse[20:28])

	if !peerExBits.GetBit(ExtensionBitLtep) {
		return HandshakeInfo{}, errors.New("peer does not support the extension protocol")
	}

	var resHash protocol.ID
	copy(resHash[:], handshakeResponse[28:48])
	if resHash != infoHash {
		return HandshakeInfo{}, errors.New("infohash mismatch")
	}

	var resPeerID protocol.ID
	copy(resPeerID[:], handshakeResponse[48:68])

	return HandshakeInfo{
		PeerID:            resPeerID,
		PeerExtensionBits: peerExBits,
	}, nil
}

type rootDict struct {
	M            mDict `bencode:"m"`
	MetadataSize int   `bencode:"metadata_size"`
}

type mDict struct {
	UTMetadata int `bencode:"ut_metadata"`
}

type extDict struct {
	MsgType int `bencode:"msg_type"`
	Piece   int `bencode:"piece"`
}

const maxMetadataSize = 10 * 1024 * 1024

func exHandshake(rw io.ReadWriter) (metadataSize uint, utMetadata uint8, err error) {
	if _, writeErr := rw.Write([]byte("\x00\x00\x00\x1a\x14\x00d1:md11:ut_metadatai1eee")); writeErr != nil {
		err = writeErr
		return
	}

	rExMessage, readErr := readExMessage(rw)
	if readErr != nil {
		err = readErr
		return
	}
	if rExMessage[1] != 0 {
		err = errors.New("first extension message is not an extension handshake")
		return
	}

	rRootDict := new(rootDict)
	if unmarshalErr := bencode.Unmarshal(rExMessage[2:], rRootDict); unmarshalErr != nil {
		err = unmarshalErr
		return
	}

	if 0 >= rRootDict.MetadataSize || rRootDict.MetadataSize >= maxMetadataSize {
		err = errors.New("metadata too big or its size is less than or equal zero")
		return
	}

	if 0 >= rRootDict.M.UTMetadata || rRootDict.M.UTMetadata >= 255 {
		err = errors.New("ut_metadata is not an uint8")
		return
	}

	return uint(rRootDict.MetadataSize), uint8(rRootDict.M.UTMetadata), nil
}

func requestAllPieces(w io.Writer, metadataSize uint, utMetadata uint8) error {
	nPieces := int(math.Ceil(float64(metadataSize) / math.Pow(2, 14)))
	for piece := range nPieces {
		extDictDump, err := bencode.Marshal(extDict{
			MsgType: 0,
			Piece:   piece,
		})
		if err != nil {
			panic(err)
		}
		if _, writeErr := fmt.Fprintf(w,
			"%s\x14%s%s",
			uintToBigEndian4(uint(2+len(extDictDump))),
			[]byte{utMetadata},
			extDictDump); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func uintToBigEndian4(i uint) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(i))
	return b
}

func readAllPieces(r io.Reader, metadataSize uint) ([]byte, error) {
	metadataBytes := make([]byte, metadataSize)
	receivedSize := uint(0)
	for receivedSize < metadataSize {
		rUmMessage, err := readUmMessage(r)
		if err != nil {
			return nil, err
		}
		rMessageBuf := bytes.NewBuffer(rUmMessage[2:])
		rExtDict := new(extDict)
		if decodeErr := bencode.NewDecoder(rMessageBuf).Decode(rExtDict); decodeErr != nil {
			return nil, decodeErr
		}
		if rExtDict.MsgType == 2 {
			return nil, errors.New("remote peer rejected sending metadataBytes")
		}
		if rExtDict.MsgType == 1 {
			metadataPiece := rMessageBuf.Bytes()
			if len(metadataPiece) > 16*1024 {
				return nil, errors.New("metadataPiece > 16kiB")
			}
			receivedSize += uint(len(metadataPiece))
			if len(metadataPiece) < 16*1024 && receivedSize != metadataSize {
				return nil, errors.New("metadataPiece < 16 kiB but incomplete")
			}
			if receivedSize > metadataSize {
				return nil, errors.New("receivedSize > metadataSize")
			}
			piece := rExtDict.Piece
			copy(
				metadataBytes[piece*int(math.Pow(2, 14)):piece*int(math.Pow(2, 14))+len(metadataPiece)],
				metadataPiece,
			)
		}
	}
	return metadataBytes, nil
}

func readExMessage(r io.Reader) ([]byte, error) {
	for {
		rMessage, err := readMessage(r)
		if err != nil {
			return nil, err
		}
		if len(rMessage) < 2 {
			continue
		}
		if rMessage[0] == byte(peer_protocol.Extended) {
			return rMessage, nil
		}
	}
}

func readUmMessage(r io.Reader) ([]byte, error) {
	for {
		rExMessage, err := readExMessage(r)
		if err != nil {
			return nil, err
		}
		if rExMessage[1] == 0x01 {
			return rExMessage, nil
		}
	}
}

func readMessage(r io.Reader) ([]byte, error) {
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBytes); err != nil {
		return nil, err
	}
	length := uint(binary.BigEndian.Uint32(lengthBytes))
	if length > maxMetadataSize {
		return nil, errors.New("message is longer than max allowed metadata size")
	}
	messageBytes := make([]byte, length)
	if _, err := io.ReadFull(r, messageBytes); err != nil {
		return nil, err
	}
	return messageBytes, nil
}
