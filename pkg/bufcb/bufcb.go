package bufcb

import "math"

type Buffer struct {
	data    []byte
	size    int
	written int
	cb      func([]byte) error
}

func NewBuffer(size int, cb func([]byte) error) *Buffer {
	b := &Buffer{
		size: size,
		data: make([]byte, size),
		cb:   cb,
	}
	return b
}

func (b *Buffer) Write(buf []byte) (int, error) {
	n := len(buf)
	if n == 0 {
		return b.Flush()
	}
	totalWritten := 0

	for n > 0 {
		err := b.cb(buf[:n])
		if err != nil {
			return b.written, err
		}
		buf = buf[n:]
		n -= int(math.Min(float64(b.size), float64(n)))
	}
	return totalWritten, nil
}

func (b *Buffer) Flush() (int, error) {
	f := b.data[:b.written]
	if len(f) > 0 {
		err := b.cb(f)
		if err != nil {
			return b.written, err
		}
	}
	b.data = f
	b.written = 0
	return 0, nil
}
