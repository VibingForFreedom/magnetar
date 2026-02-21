// Package protocol provides core BitTorrent protocol types.
// Extracted from github.com/bitmagnet-io/bitmagnet (MIT License).
package protocol

import (
	crand "crypto/rand"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/anacrolix/torrent/bencode"
)

// idClientPart encodes client identity into peer IDs.
// Azureus-style: '-', two chars for client id, four ascii digits for version, '-'.
const idClientPart = "-MG0001-"

func RandomNodeID() (id ID) {
	_, _ = crand.Read(id[:])
	return
}

// RandomNodeIDWithClientSuffix generates a node ID for the DHT client.
// Uses a suffix (not prefix) to avoid interfering with DHT distance metrics.
func RandomNodeIDWithClientSuffix() (id ID) {
	_, _ = crand.Read(id[:])
	for i := range len(idClientPart) {
		id[20-len(idClientPart)+i] = idClientPart[i]
	}
	return
}

type ID [20]byte

func ParseID(str string) (ID, error) {
	b, err := hex.DecodeString(strings.TrimPrefix(str, "0x"))
	if err != nil {
		return ID{}, err
	}
	if len(b) != 20 {
		return ID{}, errors.New("hash string must be 20 bytes")
	}
	var id ID
	copy(id[:], b)
	return id, nil
}

func MustParseID(str string) ID {
	id, err := ParseID(str)
	if err != nil {
		panic(err)
	}
	return id
}

func NewIDFromRawString(s string) (id ID) {
	if n := copy(id[:], s); n != 20 {
		panic(n)
	}
	return
}

func NewIDFromByteSlice(b []byte) (id ID, _ error) {
	if n := copy(id[:], b); n != 20 {
		return id, errors.New("must be 20 bytes")
	}
	return
}

func MustNewIDFromByteSlice(b []byte) ID {
	id, err := NewIDFromByteSlice(b)
	if err != nil {
		panic(err)
	}
	return id
}

func (id ID) String() string {
	return hex.EncodeToString(id[:])
}

func (id ID) Int160() Int160 {
	return NewInt160FromByteArray(id)
}

func (id ID) IsZero() bool {
	return id == [20]byte{}
}

func (id ID) GetBit(i int) bool {
	return id[i/8]>>(7-uint(i%8))&1 == 1 //nolint:gosec // value is within range
}

func (id ID) Bytes() []byte {
	return id[:]
}

func (id *ID) Scan(value interface{}) error {
	v, ok := value.([]byte)
	if !ok {
		return errors.New("invalid bytes type")
	}
	copy(id[:], v)
	return nil
}

func (id ID) Value() (driver.Value, error) {
	return id[:], nil
}

func (id ID) MarshalBinary() ([]byte, error) {
	return id[:], nil
}

func (id *ID) UnmarshalBinary(data []byte) error {
	if len(data) != 20 {
		return errors.New("invalid ID length")
	}
	copy(id[:], data)
	return nil
}

func (id ID) MarshalBencode() ([]byte, error) {
	return []byte("20:" + string(id[:])), nil
}

func (id *ID) UnmarshalBencode(b []byte) error {
	var s string
	if err := bencode.Unmarshal(b, &s); err != nil {
		return err
	}
	if n := copy(id[:], s); n != 20 {
		return fmt.Errorf("string has wrong length: %d", n)
	}
	return nil
}

func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	tb, err := ParseID(s)
	if err != nil {
		return err
	}
	*id = tb
	return nil
}

type MutableID ID

func (id *MutableID) SetBit(i int, v bool) {
	if v {
		id[i/8] |= 1 << (7 - uint(i%8)) //nolint:gosec // value is within range
	} else {
		id[i/8] &= ^(1 << (7 - uint(i%8))) //nolint:gosec // value is within range
	}
}

func RandomPeerID() ID {
	clientID := RandomNodeID()
	i := 0
	for _, c := range idClientPart {
		clientID[i] = byte(c)
		i++
	}
	return clientID
}
