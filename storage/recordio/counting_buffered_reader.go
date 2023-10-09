package recordio

import "io"

type Reset interface {
	Reset(r io.Reader)
}

type ByteReaderReset interface {
	io.ByteReader
	io.Reader
	Reset
	Size() int
}

type ByteReaderResetCount interface {
	ByteReaderReset
	Count() uint64
}

type CountingBufferedReader struct {
	r     ByteReaderReset
	count uint64
}

func (c *CountingBufferedReader) ReadByte() (byte, error) {
	b, err := c.r.ReadByte()
	if err == nil {
		c.count = c.count + 1
	}
	return b, err
}

func (c *CountingBufferedReader) Read(p []byte) (n int, err error) {
	read, err := c.r.Read(p)
	if err == nil {
		c.count = c.count + uint64(read)
	}
	return read, err
}

func (c *CountingBufferedReader) Reset(r io.Reader) {
	c.r.Reset(r)
}

func (c *CountingBufferedReader) Count() uint64 {
	return c.count
}

func (c *CountingBufferedReader) Size() int {
	return c.r.Size()
}

func NewCountingByteReader(reader ByteReaderReset) ByteReaderResetCount {
	return &CountingBufferedReader{r: reader, count: 0}
}
