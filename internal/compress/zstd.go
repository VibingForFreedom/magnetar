package compress

import (
	"bytes"
	"io"
	"sync"

	"github.com/klauspost/compress/zstd"
)

var (
	encoderPool = sync.Pool{
		New: func() interface{} {
			w, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
			return w
		},
	}
	decoderPool = sync.Pool{
		New: func() interface{} {
			r, _ := zstd.NewReader(nil)
			return r
		},
	}
	bufPool = sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
)

func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	enc := encoderPool.Get().(*zstd.Encoder)
	defer encoderPool.Put(enc)

	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)

	enc.Reset(buf)

	_, err := enc.Write(data)
	if err != nil {
		return nil, err
	}

	err = enc.Close()
	if err != nil {
		return nil, err
	}

	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	return result, nil
}

func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	dec := decoderPool.Get().(*zstd.Decoder)
	defer decoderPool.Put(dec)

	err := dec.Reset(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	result, err := io.ReadAll(dec)
	if err != nil {
		return nil, err
	}

	return result, nil
}
