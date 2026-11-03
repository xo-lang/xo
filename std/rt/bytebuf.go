package rt

import "bytes"

// ByteBuffer is decision 0480's buffer: bytes.Buffer, with each record a
// method returns copied into a Bytes (bytes.Buffer's Next is valid only
// until the next write; an Xo view stays valid). Move only (core 5.3).
type ByteBuffer struct {
	b *bytes.Buffer
	// the unread bytes M_read_until searched for delim without finding it
	scanned int
	delim   int
}

// ByteBuffer_new is `ByteBuffer.new()`.
func ByteBuffer_new(*Task) ByteBuffer { return ByteBuffer{delim: -1} }

// ByteBuffer_with_capacity is `ByteBuffer.with_capacity(n)`.
func ByteBuffer_with_capacity(_ *Task, n int64) ByteBuffer {
	if n <= 0 {
		return ByteBuffer{delim: -1}
	}
	return ByteBuffer{b: bytes.NewBuffer(make([]byte, 0, n)), delim: -1}
}

func (b *ByteBuffer) buf() *bytes.Buffer {
	if b.b == nil {
		b.b = new(bytes.Buffer)
	}
	return b.b
}

// M_write is `b.write(data)`.
func (b *ByteBuffer) M_write(_ *Task, data Bytes) { b.buf().WriteString(string(data)) }

// M_len is `b.len()`.
func (b *ByteBuffer) M_len(*Task) int64 {
	if b.b == nil {
		return 0
	}
	return int64(b.b.Len())
}

func (b *ByteBuffer) clamp(n int64) int {
	m := int64(0)
	if b.b != nil {
		m = int64(b.b.Len())
	}
	return int(min(max(n, 0), m))
}

func (b *ByteBuffer) consumed(k int) { b.scanned = max(b.scanned-k, 0) }

// M_peek is `b.peek(n)`.
func (b *ByteBuffer) M_peek(_ *Task, n int64) Bytes {
	k := b.clamp(n)
	if k == 0 {
		return ""
	}
	return Bytes(b.b.Bytes()[:k])
}

// M_next is `b.next(n)`.
func (b *ByteBuffer) M_next(_ *Task, n int64) Bytes {
	k := b.clamp(n)
	if k == 0 {
		return ""
	}
	b.consumed(k)
	return Bytes(b.b.Next(k))
}

// M_discard is `b.discard(n)`.
func (b *ByteBuffer) M_discard(_ *Task, n int64) {
	if k := b.clamp(n); k > 0 {
		b.consumed(k)
		b.b.Next(k)
	}
}

// M_read_until is `b.read_until(delim)`: through delim, consumed; None when
// the unread bytes hold none (nothing consumed).
func (b *ByteBuffer) M_read_until(_ *Task, delim uint8) Option[Bytes] {
	if b.b == nil {
		return None[Bytes]()
	}
	from := 0
	if b.delim == int(delim) {
		from = b.scanned
	}
	u := b.b.Bytes()
	i := bytes.IndexByte(u[from:], delim)
	if i < 0 {
		b.delim, b.scanned = int(delim), len(u)
		return None[Bytes]()
	}
	k := from + i + 1
	b.consumed(k)
	return Some(Bytes(b.b.Next(k)))
}

// M_find is `b.find(sub)`.
func (b *ByteBuffer) M_find(_ *Task, sub Bytes) Option[int64] {
	var u []byte
	if b.b != nil {
		u = b.b.Bytes()
	}
	if i := bytes.Index(u, []byte(sub)); i >= 0 {
		return Some(int64(i))
	}
	return None[int64]()
}
