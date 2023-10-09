package sstables

import (
	"os"

	"github.com/THPTUHA/kairos/storage/recordio"
	rProto "github.com/THPTUHA/kairos/storage/recordio/proto"
	"github.com/THPTUHA/kairos/storage/skiplist"
	sProto "github.com/THPTUHA/kairos/storage/sstables/proto"
	"github.com/steakknife/bloomfilter"
)

type SSTableWriterOptions struct {
	basePath                      string
	indexCompressionType          int
	dataCompressionType           int
	enableBloomFilter             bool
	bloomExpectedNumberOfElements uint64
	bloomFpProbability            float64
	writeBufferSizeBytes          int
	keyComparator                 skiplist.Comparator[[]byte]
}

type SSTableStreamWriter struct {
	opts *SSTableWriterOptions

	indexFilePath string
	dataFilePath  string
	metaFilePath  string

	indexWriter  rProto.WriterI
	dataWriter   recordio.WriterI
	metaDataFile *os.File

	bloomFilter *bloomfilter.Filter
	metaData    *sProto.MetaData

	lastKey []byte
}
