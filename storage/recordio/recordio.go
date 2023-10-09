package recordio

import (
	"fmt"
	"os"

	"github.com/THPTUHA/kairos/storage/recordio/compressor"
)

const Version1 uint32 = 0x01
const Version2 uint32 = 0x02
const CurrentVersion = Version2
const MagicNumberSeparator uint32 = 0x130691
const MagicNumberSeparatorLong uint64 = 0x130691

// FileHeaderSizeBytes has a 4 byte version number, 4 byte compression code = 8 bytes
const FileHeaderSizeBytes = 8

const DefaultBufferSize = 1024 * 1024 * 4

const (
	CompressionTypeNone = iota
	CompressionTypeGZIP
	CompressionTypeSnappy
	CompressionTypeLzw
)

type SizeI interface {
	Size() uint64
}

type CloseableI interface {
	Close() error
}

type OpenableI interface {
	Open() error
}

type OpenClosableI interface {
	CloseableI
	OpenableI
}

type WriterI interface {
	OpenClosableI
	SizeI
	Write(record []byte) (uint64, error)
	WriteSync(record []byte) (uint64, error)
}

type ReaderI interface {
	OpenClosableI
	ReadNext() ([]byte, error)
	SkipNext() error
}

type ReadAtI interface {
	OpenClosableI
	ReadNextAt(offset uint64) ([]byte, error)
}

type ReaderWriterCloserFactory interface {
	CreateNewReader(filePath string, bufSize int) (*os.File, ByteReaderResetCount, error)
	CreateNewWriter(filePath string, bufSize int) (*os.File, WriteCloserFlusher, error)
}

func NewCompressorForType(compType int) (compressor.CompressionI, error) {
	switch compType {
	case CompressionTypeNone:
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported compression type %d", compType)
	}
}
