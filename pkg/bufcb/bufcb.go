package bufcb

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
	if len(buf) == 0 {
		return b.Flush()
	}
	totalWritten := 0

	for _, e := range buf {
		b.data[b.written] = e
		b.written++
		if b.written == b.size {
			err := b.cb(b.data)
			if err != nil {
				return totalWritten, err
			}
			b.written = 0
		}
		totalWritten++
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
	b.written = 0
	return 0, nil
}
