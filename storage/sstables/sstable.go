package sstables

import (
	"github.com/THPTUHA/kairos/storage/skiplist"
	"github.com/THPTUHA/kairos/storage/sstables/proto"
)

var IndexFileName = "index.rio"
var DataFileName = "data.rio"
var BloomFileName = "bloom.bf.gz"
var MetaFileName = "meta.pb.bin"

type SSTableIteratorI interface {
	Next() ([]byte, []byte, error)
}

type SSTableReaderI interface {
	Contains(key []byte) bool
	Get(key []byte) ([]byte, error)
	Scan() (SSTableIteratorI, error)
	ScanStartingAt(key []byte) (SSTableIteratorI, error)
	ScanRange(keyLower []byte, keyHigher []byte) (SSTableIteratorI, error)
	Close() error
	MetaData() *proto.MetaData
	BasePath() string
}

type SSTableSimpleWriterI interface {
	WriteSkipList(skipListMap *skiplist.MapI[[]byte, []byte]) error
}

type SSTableStreamWriterI interface {
	Open() error
	WriteNext(key []byte, value []byte) error
	Close() error
}
