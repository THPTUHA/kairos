package sstables

import (
	"github.com/THPTUHA/kairos/storage/recordio"
	rProto "github.com/THPTUHA/kairos/storage/recordio/proto"
	"github.com/THPTUHA/kairos/storage/skiplist"
	"github.com/THPTUHA/kairos/storage/sstables/proto"
	"github.com/steakknife/bloomfilter"
)

type SSTableReaderOptions struct {
	basePath      string
	keyComparator skiplist.Comparator[[]byte]
}

type SSTableReader struct {
	opts          *SSTableReaderOptions
	bloomFilter   *bloomfilter.Filter
	keyComparator skiplist.Comparator[[]byte]
	index         skiplist.MapI[[]byte, uint64]
	v0DataReader  rProto.ReadAtI
	dataReader    recordio.ReadAtI
	metaData      *proto.MetaData
	miscClosers   []recordio.CloseableI
}
