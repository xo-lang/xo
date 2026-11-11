package rt

import "hash/adler32"

// std/adler32's update on the Go backend (decision 0597 part 2: declared
// bodyless in std/adler32.xo, the native backends run
// std/native/adler32_native.xo's): Go's hash/adler32 itself. checksum is
// generated from std/adler32.xo.

// Adler32_update is `adler32.update(sum, data)`.
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
