package compressor

type CompressionI interface {
	Compress(record []byte) ([]byte, error)
	Decompress(buf []byte) ([]byte, error)
	CompressWithBuf(record []byte, destinationBuffer []byte) ([]byte, error)
	DecompressWithBuf(buf []byte, destinationBuffer []byte) ([]byte, error)
}
