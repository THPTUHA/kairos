package readerpool

import (
	"bytes"
	"strings"
	"sync"
)

var stringReaderPool sync.Pool

func GetStringReader(data string) *strings.Reader {
	r := bytesReaderPool.Get()
	if r == nil {
		return strings.NewReader(data)
	}
	reader := r.(*strings.Reader)
	reader.Reset(data)
	return reader
}

func PutStringReader(reader *strings.Reader) {
	reader.Reset("")
	stringReaderPool.Put(reader)
}

var bytesReaderPool sync.Pool

func GetBytesReader(data []byte) *bytes.Reader {
	r := bytesReaderPool.Get()
	if r == nil {
		return bytes.NewReader(data)
	}
	reader := r.(*bytes.Reader)
	reader.Reset(data)
	return reader
}

func PutBytesReader(reader *bytes.Reader) {
	reader.Reset(nil)
	bytesReaderPool.Put(reader)
}
