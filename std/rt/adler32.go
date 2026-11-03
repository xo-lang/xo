package rt

import "hash/adler32"

// std/adler32 on the Go backend: Go's hash/adler32 itself;
// std/native/adler32.xo ports it.

// Adler32_checksum is `adler32.checksum(data)`.
func Adler32_checksum(_ *Task, data Bytes) int64 {
	return int64(adler32.Checksum([]byte(data)))
}

// Adler32_update is `adler32.update(sum, data)`: Go has no Update, so
// the digest is restored at sum through its marshaled state.
func Adler32_update(_ *Task, sum int64, data Bytes) int64 {
	h := adler32.New()
	st, _ := h.(interface{ UnmarshalBinary([]byte) error })
	b := []byte("adl\x01")
	b = append(b, byte(sum>>24), byte(sum>>16), byte(sum>>8), byte(sum))
	if err := st.UnmarshalBinary(b); err != nil {
		panic(err)
	}
	h.Write([]byte(data))
	return int64(h.Sum32())
}
