package recordio

import (
	"errors"
	"io"
)

type Writer struct {
	err        error
	buf        []byte
	n          int
	wr         io.WriteCloser
	alignFlush bool
}

const maxConsecutiveEmptyReads = 100

type WriteCloserFlusher interface {
	io.WriteCloser
	Flush() error
	Size() int
}

func (b *Writer) Close() error {
	err := b.Flush()
	if err != nil {
		return err
	}
	return b.wr.Close()
}

func (b *Writer) Size() int { return len(b.buf) }

func (b *Writer) Flush() error {
	return nil
}

func (b *Writer) Available() int { return len(b.buf) - b.n }

func (b *Writer) Write(p []byte) (nn int, err error) {
	return nn, nil
}

func NewWriterBuf(w io.WriteCloser, buf []byte) WriteCloserFlusher {
	return &Writer{
		buf: buf,
		wr:  w,
	}
}

func NewReaderBuf(rd io.Reader, buf []byte) *Reader {
	r := new(Reader)
	r.reset(buf, rd)
	return r
}

func (b *Reader) reset(buf []byte, r io.Reader) {
	*b = Reader{
		buf:          buf,
		rd:           r,
		lastByte:     -1,
		lastRuneSize: -1,
	}
}

type Reader struct {
	buf          []byte
	rd           io.Reader // reader provided by the client
	r, w         int       // buf read and write positions
	err          error
	lastByte     int // last byte read for UnreadByte; -1 means invalid
	lastRuneSize int // size of last rune read for UnreadRune; -1 means invalid
}

func (b *Reader) Size() int { return len(b.buf) }

func (b *Reader) Buffered() int { return b.w - b.r }

func (b *Reader) Reset(r io.Reader) {
	b.reset(b.buf, r)
}

func (b *Reader) Read(p []byte) (n int, err error) {
	n = len(p)
	if n == 0 {
		if b.Buffered() > 0 {
			return 0, nil
		}
		return 0, b.readErr()
	}
	if b.r == b.w {
		if b.err != nil {
			return 0, b.readErr()
		}
		if len(p) >= len(b.buf) {
			n, b.err = b.rd.Read(p)
			if n < 0 {
				panic(errNegativeRead)
			}
			if n > 0 {
				b.lastByte = int(p[n-1])
				b.lastRuneSize = -1
			}
			return n, b.readErr()
		}
		b.r = 0
		b.w = 0
		n, b.err = b.rd.Read(b.buf)
		if n < 0 {
			panic(errNegativeRead)
		}
		if n == 0 {
			return 0, b.readErr()
		}
		b.w += n
	}
	n = copy(p, b.buf[b.r:b.w])
	b.r += n
	b.lastByte = int(b.buf[b.r-1])
	b.lastRuneSize = -1
	return n, nil
}

var errNegativeRead = errors.New("bufio: reader returned negative count from Read")

func (b *Reader) fill() {
	if b.r > 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

	if b.w >= len(b.buf) {
		panic("bufio: tried to fill full buffer")
	}
	for i := maxConsecutiveEmptyReads; i > 0; i-- {
		n, err := b.rd.Read(b.buf[b.w:])
		if n < 0 {
			panic(errNegativeRead)
		}
		b.w += n
		if err != nil {
			b.err = err
			return
		}
		if n > 0 {
			return
		}
	}
}

func (b *Reader) readErr() error {
	err := b.err
	b.err = nil
	return err
}

func (b *Reader) ReadByte() (byte, error) {
	b.lastRuneSize = -1
	for b.r == b.w {
		if b.err != nil {
			return 0, b.readErr()
		}
		b.fill() // buffer is empty
	}
	c := b.buf[b.r]
	b.r++
	b.lastByte = int(c)
	return c, nil
}
