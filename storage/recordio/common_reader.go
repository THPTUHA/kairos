package recordio

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/THPTUHA/kairos/storage/recordio/compressor"
	pool "github.com/libp2p/go-buffer-pool"
)

type Header struct {
	compressionType int
	compressor      compressor.CompressionI
	fileVersion     uint32
}

var MagicNumberMismatchErr = fmt.Errorf("magic number mismatch")

func readFileHeaderFromBuffer(buffer []byte) (*Header, error) {
	if len(buffer) != FileHeaderSizeBytes {
		return nil, fmt.Errorf("file header buffer size mismatch, expected %d but was %d", FileHeaderSizeBytes, len(buffer))
	}
	fileVersion := binary.LittleEndian.Uint32(buffer[0:4])
	if fileVersion > CurrentVersion || fileVersion < Version1 {
		return nil, fmt.Errorf("version mismatch, expected a value from %d to %d but was %d", Version1, CurrentVersion, fileVersion)
	}

	compressionType := binary.LittleEndian.Uint32(buffer[4:8])
	if compressionType > CompressionTypeLzw {
		return nil, fmt.Errorf("unknown compression type [%d]", compressionType)
	}
	header := &Header{compressionType: int(compressionType), fileVersion: fileVersion}

	cmp, err := NewCompressorForType(header.compressionType)
	if err != nil {
		return nil, err
	}
	header.compressor = cmp
	return header, nil
}

func readRecordHeader(r io.ByteReader) (uint64, uint64, error) {
	magicNumber, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, 0, err
	}

	if magicNumber != MagicNumberSeparatorLong {
		return 0, 0, MagicNumberMismatchErr
	}

	payloadSizeUncompressed, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, 0, err
	}

	payloadSizeCompressed, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, 0, err
	}

	return payloadSizeUncompressed, payloadSizeCompressed, nil
}

func allocateRecordBufferPooled(bufferPool *pool.BufferPool, header *Header, payloadSizeUncompressed uint64, payloadSizeCompressed uint64) (uint64, []byte) {
	expectedBytesRead := payloadSizeUncompressed
	if header.compressor != nil {
		expectedBytesRead = payloadSizeCompressed
	}

	return expectedBytesRead, bufferPool.Get(int(expectedBytesRead))
}
