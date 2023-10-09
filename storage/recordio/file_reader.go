package recordio

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	pool "github.com/libp2p/go-buffer-pool"
)

type FileReader struct {
	open   bool
	closed bool

	currentOffset uint64
	file          *os.File
	header        *Header
	reader        ByteReaderResetCount
	bufferPool    *pool.BufferPool
}

func (r *FileReader) Open() error {
	if r.open {
		return fmt.Errorf("file reader for '%s' is already opened", r.file.Name())
	}

	if r.closed {
		return fmt.Errorf("file reader for '%s' is already closed", r.file.Name())
	}

	bytes := make([]byte, FileHeaderSizeBytes)
	numRead, err := io.ReadFull(r.reader, bytes)
	if err != nil {
		return fmt.Errorf("error while reading header bytes of '%s': %w", r.file.Name(), err)
	}

	if numRead != len(bytes) {
		return fmt.Errorf("not enough bytes found in the header, expected %d but were %d", len(bytes), numRead)
	}

	r.header, err = readFileHeaderFromBuffer(bytes)
	if err != nil {
		return fmt.Errorf("error while parsing header of '%s': %w", r.file.Name(), err)
	}

	r.currentOffset = uint64(len(bytes))

	r.bufferPool = new(pool.BufferPool)
	r.open = true

	return nil
}

func (r *FileReader) ReadNext() ([]byte, error) {
	if !r.open || r.closed {
		return nil, fmt.Errorf("file reader for '%s' was either not opened yet or is closed already", r.file.Name())
	}
	start := r.reader.Count()
	payloadSizeUncompressed, payloadSizeCompressed, err := readRecordHeader(r.reader)
	if err != nil {
		// due to the use of blocked writes in DirectIO, we need to test whether the remainder of the file contains only zeros.
		// This would indicate a properly written file and the actual end - and not a malformed record.
		if errors.Is(err, MagicNumberMismatchErr) {
			remainder, err := ioutil.ReadAll(r.reader)
			if err != nil {
				return nil, fmt.Errorf("error while parsing record header seeking for file end of '%s': %w", r.file.Name(), err)
			}
			for _, b := range remainder {
				if b != 0 {
					return nil, fmt.Errorf("error while parsing record header for zeros towards the file end of '%s': %w", r.file.Name(), MagicNumberMismatchErr)
				}
			}

			// no other bytes than zeros have been read so far, that must've been the valid end of the file.
			return nil, io.EOF
		}

		return nil, fmt.Errorf("error while parsing record header of '%s': %w", r.file.Name(), err)
	}
	expectedBytesRead, pooledRecordBuffer := allocateRecordBufferPooled(r.bufferPool, r.header, payloadSizeUncompressed, payloadSizeCompressed)
	numRead, err := io.ReadFull(r.reader, pooledRecordBuffer)
	if err != nil {
		return nil, fmt.Errorf("error while reading into record buffer of '%s': %w", r.file.Name(), err)
	}

	if uint64(numRead) != expectedBytesRead {
		return nil, fmt.Errorf("not enough bytes in the record of '%s' found, expected %d but were %d", r.file.Name(), expectedBytesRead, numRead)
	}

	var returnSlice []byte

	if r.header.compressor != nil {
		pooledDecompressionBuffer := r.bufferPool.Get(int(payloadSizeUncompressed))
		decompressedRecord, err := r.header.compressor.DecompressWithBuf(pooledRecordBuffer, pooledDecompressionBuffer)
		if err != nil {
			return nil, err
		}
		returnSlice = make([]byte, len(decompressedRecord))
		copy(returnSlice, decompressedRecord)
		r.bufferPool.Put(pooledRecordBuffer)
		r.bufferPool.Put(pooledDecompressionBuffer)
	} else {
		returnSlice = make([]byte, len(pooledRecordBuffer))
		copy(returnSlice, pooledRecordBuffer)
		r.bufferPool.Put(pooledRecordBuffer)
	}

	r.currentOffset = r.currentOffset + (r.reader.Count() - start)
	return returnSlice, nil
}

func (r *FileReader) SkipNext() error {
	if !r.open || r.closed {
		return fmt.Errorf("file reader for '%s' was either not opened yet or is closed already", r.file.Name())
	}

	start := r.reader.Count()
	payloadSizeUncompressed, payloadSizeCompressed, err := readRecordHeader(r.reader)
	if err != nil {
		return fmt.Errorf("error while reading record header of '%s': %w", r.file.Name(), err)
	}

	expectedBytesSkipped := payloadSizeUncompressed
	if r.header.compressor != nil {
		expectedBytesSkipped = payloadSizeCompressed
	}

	expectedOffset := int64(r.currentOffset + expectedBytesSkipped + (r.reader.Count() - start))
	newOffset, err := r.file.Seek(expectedOffset, 0)
	if err != nil {
		return fmt.Errorf("error while seeking to offset %d in '%s': %w", expectedOffset, r.file.Name(), err)
	}

	if newOffset != expectedOffset {
		return fmt.Errorf("seeking in '%s' did not return expected offset %d, it was %d", r.file.Name(), expectedOffset, newOffset)
	}

	r.reader.Reset(r.file)
	r.currentOffset = uint64(newOffset)
	return nil
}

func (r *FileReader) Close() error {
	r.closed = true
	r.open = false
	return r.file.Close()
}

func NewFileReaderWithPath(path string) (ReaderI, error) {
	return newFileReaderWithFactory(path, BufferedIOFactory{})
}

func NewFileReaderWithFile(file *os.File) (ReaderI, error) {
	err := file.Close()
	if err != nil {
		return nil, fmt.Errorf("error while closing existing file handle at '%s': %w", file.Name(), err)
	}

	return newFileReaderWithFactory(file.Name(), BufferedIOFactory{})
}

func newFileReaderWithFactory(path string, factory ReaderWriterCloserFactory) (ReaderI, error) {
	f, r, err := factory.CreateNewReader(path, DefaultBufferSize)
	if err != nil {
		return nil, err
	}

	return &FileReader{
		file:          f,
		reader:        r,
		open:          false,
		closed:        false,
		currentOffset: 0,
	}, nil
}
