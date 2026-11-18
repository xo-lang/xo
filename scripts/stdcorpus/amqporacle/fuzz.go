package main

// The fuzz mode (internal/stdtest TestAmqpFuzz, task #115): `fuzz SEED N` prints N cases, each
// "== " then "c HEX" (a message's bytes) and "w TEXT" (go-amqp's UnmarshalBinary of them shown
// as showMsg, "error" when it refuses them, "panic" when it panics, "unshowable" when show has
// no case for what it decoded). The bytes are go-amqp's MarshalBinary of a seeded message, most
// of them then mutated: a byte set to a constructor code or a random value, a cut, inserted
// bytes, a section repeated or two swapped.

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	amqp "github.com/Azure/go-amqp"
)

var fzCodes = []byte{0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x60, 0x61,
	0x70, 0x71, 0x72, 0x73, 0x80, 0x81, 0x82, 0x83, 0x84, 0x94, 0x98, 0xa0, 0xa1, 0xa3, 0xb0, 0xb1, 0xb3,
	0xc0, 0xc1, 0xd0, 0xd1, 0xe0, 0xf0, 0x00, 0x01, 0xff, 0x77, 0x75, 0x76}

func fzStr(r *rand.Rand) string {
	pool := []string{"", "a", "héllo", "世界", "x\x00y", "\U0001F600", "key", "amqp:x", strings.Repeat("s", 300)}
	if r.Intn(4) == 0 {
		b := make([]byte, r.Intn(12))
		for i := range b {
			b[i] = byte(32 + r.Intn(95))
		}
		return string(b)
	}
	return pool[r.Intn(len(pool))]
}

func fzScalar(r *rand.Rand) any {
	switch r.Intn(19) {
	case 0:
		return nil
	case 1:
		return r.Intn(2) == 0
	case 2:
		return uint8(r.Intn(256))
	case 3:
		return uint16(r.Intn(65536))
	case 4:
		return []uint32{0, 1, 255, 256, math.MaxUint32}[r.Intn(5)]
	case 5:
		return []uint64{0, 1, 255, 256, math.MaxUint64, 1 << 40}[r.Intn(6)]
	case 6:
		return int8(r.Intn(256) - 128)
	case 7:
		return int16(r.Intn(65536) - 32768)
	case 8:
		return []int32{0, -1, 127, -128, 128, math.MaxInt32, math.MinInt32}[r.Intn(7)]
	case 9:
		return []int64{0, -1, 127, -128, 1 << 40, math.MaxInt64, math.MinInt64}[r.Intn(7)]
	case 10:
		return []float32{0, -1.5, float32(math.Inf(1)), float32(math.NaN())}[r.Intn(4)]
	case 11:
		return []float64{0, 0.1, math.Inf(-1), math.NaN(), math.MaxFloat64}[r.Intn(5)]
	case 12, 13:
		return fzStr(r)
	case 14:
		return amqp.Symbol(fzStr(r))
	case 15:
		b := make([]byte, []int{0, 1, 5, 255, 256}[r.Intn(5)])
		r.Read(b)
		return b
	case 16:
		return time.UnixMilli([]int64{0, -1, 1700000000123}[r.Intn(3)]).UTC()
	case 17:
		var u amqp.UUID
		r.Read(u[:])
		return u
	default:
		return []any{[]int32{1, -2}, []string{"a", "bc"}, []amqp.Symbol{"x"}, []uint64{7}, []bool{true}}[r.Intn(5)]
	}
}

func fzValue(r *rand.Rand, depth int) any {
	if depth > 0 && r.Intn(4) == 0 {
		if r.Intn(2) == 0 {
			n := r.Intn(4)
			l := make([]any, n)
			for i := range l {
				l[i] = fzValue(r, depth-1)
			}
			return l
		}
		m := map[any]any{}
		for i, n := 0, r.Intn(4); i < n; i++ {
			k := fzScalar(r)
			switch k.(type) {
			case []byte, []any, []int32, []string, []amqp.Symbol, []uint64, []bool:
				k = fzStr(r)
			}
			if f, ok := k.(float64); ok && f != f {
				k = "nan"
			}
			if f, ok := k.(float32); ok && f != f {
				k = "nan32"
			}
			m[k] = fzValue(r, depth-1)
		}
		return m
	}
	return fzScalar(r)
}

func fzAnn(r *rand.Rand) amqp.Annotations {
	if r.Intn(2) == 0 {
		return nil
	}
	a := amqp.Annotations{}
	for i, n := 0, r.Intn(3); i < n; i++ {
		if r.Intn(4) == 0 {
			a[int64(r.Intn(5))] = fzValue(r, 1)
		} else {
			a["x-"+fzStr(r)] = fzValue(r, 1)
		}
	}
	return a
}

func fzMessage(r *rand.Rand) *amqp.Message {
	m := &amqp.Message{}
	if r.Intn(2) == 0 {
		m.Header = &amqp.MessageHeader{Durable: r.Intn(2) == 0, Priority: uint8(r.Intn(10)),
			TTL: time.Duration(r.Intn(100000)) * time.Millisecond, FirstAcquirer: r.Intn(2) == 0, DeliveryCount: uint32(r.Intn(5))}
	}
	m.DeliveryAnnotations = fzAnn(r)
	m.Annotations = fzAnn(r)
	if r.Intn(2) == 0 {
		p := &amqp.MessageProperties{}
		if r.Intn(2) == 0 {
			p.MessageID = []any{"id-1", uint64(7), []byte{1, 2}, amqp.UUID{1}}[r.Intn(4)]
		}
		if r.Intn(3) == 0 {
			p.UserID = []byte("user")
		}
		if r.Intn(2) == 0 {
			p.To = sp(fzStr(r))
		}
		if r.Intn(3) == 0 {
			p.Subject = sp(fzStr(r))
		}
		if r.Intn(3) == 0 {
			p.ContentType = sp("application/json")
		}
		if r.Intn(3) == 0 {
			p.CreationTime = tp(1700000000123)
		}
		if r.Intn(4) == 0 {
			p.GroupSequence = up(uint32(r.Intn(1000)))
		}
		if r.Intn(4) == 0 {
			p.CorrelationID = "corr"
		}
		m.Properties = p
	}
	if r.Intn(2) == 0 {
		ap := map[string]any{}
		for i, n := 0, r.Intn(4); i < n; i++ {
			ap[fzStr(r)] = fzScalar(r)
		}
		m.ApplicationProperties = ap
	}
	switch r.Intn(4) {
	case 0:
		for i, n := 0, 1+r.Intn(3); i < n; i++ {
			b := make([]byte, r.Intn(40))
			r.Read(b)
			m.Data = append(m.Data, b)
		}
	case 1:
		for i, n := 0, 1+r.Intn(2); i < n; i++ {
			m.Sequence = append(m.Sequence, []any{fzScalar(r), fzStr(r)})
		}
	case 2:
		m.Value = fzValue(r, 2)
		if m.Value == nil {
			m.Value = "v"
		}
	default:
		m.Data = [][]byte{[]byte("body")}
	}
	if r.Intn(4) == 0 {
		m.Footer = fzAnn(r)
	}
	return m
}

func fzMutate(r *rand.Rand, b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	b = bytes.Clone(b)
	for k, n := 0, 1+r.Intn(3)/2; k < n && len(b) > 0; k++ {
		i := r.Intn(len(b))
		switch r.Intn(7) {
		case 0, 1:
			b[i] = fzCodes[r.Intn(len(fzCodes))]
		case 2:
			b[i] = byte(r.Intn(256))
		case 3:
			b = b[:i]
		case 4:
			ins := make([]byte, 1+r.Intn(4))
			r.Read(ins)
			b = append(b[:i:i], append(ins, b[i:]...)...)
		case 5:
			// a length or count byte near a constructor
			b[i] = []byte{0, 1, 2, 0x7f, 0x80, 0xff}[r.Intn(6)]
		default:
			// repeat a tail (a section again, or part of one)
			b = append(b, b[i:]...)
		}
	}
	return b
}

// fzIdx and fzWhy: the case decoding, and AMQPFUZZ_WHY set (go-amqp's errors to stderr).
var (
	fzIdx int
	fzWhy = os.Getenv("AMQPFUZZ_WHY") != ""
)

// fzBadUTF8: show met a string that is not UTF-8 (fzNote is a no-op outside the fuzz mode).
var fzBadUTF8 bool

func fzNote(s string) {
	if !utf8.ValidString(s) {
		fzBadUTF8 = true
	}
}

// fzSections: the section codes of b, which go-amqp decodes, found by decoding its prefixes
// that end where a described section could start (00 53 CODE, or 00 80 and an 8 byte code).
func fzSections(b []byte) []byte {
	var out []byte
	for _, p := range fzSectionAt(b) {
		out = append(out, fzCode(b, p))
	}
	return out
}

// fzCode is the section code of the descriptor at p (00 53 CODE, or 00 80 and an 8 byte code).
func fzCode(b []byte, p int) byte {
	if b[p+1] == 0x53 {
		return b[p+2]
	}
	return b[p+9]
}

// fzSectionAt: where fzSections' sections start.
func fzSectionAt(b []byte) []int {
	described := func(p int) bool {
		return p+2 < len(b) && b[p] == 0 && b[p+1] == 0x53 || p+9 < len(b) && b[p] == 0 && b[p+1] == 0x80
	}
	// a prefix that ends where go-amqp reads a section can still fail on its own: a list32 or
	// map32 whose size runs past the prefix (go-amqp checks a size against what is left, then
	// reads the fields by count), so each prefix is also tried followed by a data section long
	// enough for any size in it (TestAmqpFuzz seed 104, 2026-11-18); the pad must then be read
	// as that data section, not eaten by the fields of a section the prefix cuts (seed 107)
	pad := append([]byte{0, 0x53, 0x75, 0xb0, 0, 0, 0, 0}, make([]byte, len(b))...)
	binary.BigEndian.PutUint32(pad[4:8], uint32(len(b)))
	var out []int
	for p := 0; p < len(b); p++ {
		if !described(p) {
			continue
		}
		if p > 0 {
			var m, n amqp.Message
			if m.UnmarshalBinary(b[:p]) != nil && (n.UnmarshalBinary(append(b[:p:p], pad...)) != nil || !fzPadIsData(&n, len(b))) {
				continue
			}
		}
		out = append(out, p)
	}
	return out
}

// fzPadIsData: m's last data section is fzSectionAt's pad (n zero bytes).
func fzPadIsData(m *amqp.Message, n int) bool {
	if len(m.Data) == 0 {
		return false
	}
	d := m.Data[len(m.Data)-1]
	return len(d) == n && bytes.Count(d, []byte{0}) == n
}

// fzTTLPastDuration: a header section with a long or ulong (0x80, 0x81) past what time.Duration
// holds in milliseconds. go-amqp's MessageHeader.TTL is a time.Duration (the ttl times
// time.Millisecond), which wraps there; std/amqp keeps the wire value. The only header fields
// that can be 8 bytes wide are ttl and delivery-count, and go-amqp refuses a delivery-count past
// a uint32, so a decoded header with such a value has an unrepresentable ttl.
func fzTTLPastDuration(b []byte) bool {
	at := fzSectionAt(b)
	for i, p := range at {
		if fzCode(b, p) != 0x70 {
			continue
		}
		end := len(b)
		if i+1 < len(at) {
			end = at[i+1]
		}
		for q := p + 3; q+9 <= end; q++ {
			if b[q] != 0x80 && b[q] != 0x81 {
				continue
			}
			v := uint64(0)
			for _, x := range b[q+1 : q+9] {
				v = v<<8 | uint64(x)
			}
			lim := uint64(math.MaxInt64 / int64(time.Millisecond))
			if b[q] == 0x80 && v > lim || b[q] == 0x81 && (int64(v) > int64(lim) || int64(v) < -int64(lim)) {
				return true
			}
		}
	}
	return false
}

// fzDroppedBadUTF8: a str or sym that is not UTF-8 in a section go-amqp
// decodes and then drops (a second delivery annotations, annotations or
// application properties section replaces the first), which showMsg never
// sees; std/amqp decodes every section and refuses it (TestAmqpFuzz seed
// 21, 2026-11-17). Each section is decoded on its own and shown.
func fzDroppedBadUTF8(b []byte) bool {
	at := fzSectionAt(b)
	bad := false
	for i, p := range at {
		end := len(b)
		if i+1 < len(at) {
			end = at[i+1]
		}
		var m amqp.Message
		if m.UnmarshalBinary(b[p:end]) != nil {
			continue
		}
		fzBadUTF8 = false
		func() {
			defer func() { recover() }()
			showMsg(&m)
		}()
		if fzBadUTF8 {
			bad = true
		}
	}
	fzBadUTF8 = false
	return bad
}

// fzRefused: std/amqp's documented departures from go-amqp, which accepts every one of them: a
// str or sym that is not UTF-8 (Xo's Str is UTF-8; go-amqp keeps the bytes), a second header or
// properties section, body sections of two kinds or a second value section (go-amqp keeps the
// last header, properties and value, and every data and sequence section), and a section
// descriptor in the ulong form whose code is past a byte (go-amqp reads its last byte).
func fzRefused(b []byte) bool {
	if fzBadUTF8 {
		return true
	}
	// a section whose descriptor is the 8 byte ulong form with a code past a byte: go-amqp's
	// PeekMessageType reads only its last byte, std/amqp the whole code
	for p := 0; p+9 < len(b); p++ {
		if b[p] == 0 && b[p+1] == 0x80 && !bytes.Equal(b[p+2:p+9], make([]byte, 7)) {
			if p == 0 {
				return true
			}
			var m amqp.Message
			if m.UnmarshalBinary(b[:p]) == nil {
				return true
			}
		}
	}
	header, props, kind := false, false, byte(0)
	for _, c := range fzSections(b) {
		switch c {
		case 0x70:
			if header {
				return true
			}
			header = true
		case 0x73:
			if props {
				return true
			}
			props = true
		case 0x75, 0x76, 0x77:
			if kind != 0 && (kind != c || c == 0x77) {
				return true
			}
			kind = c
		}
	}
	return false
}

func fzDecode(b []byte) (out string) {
	defer func() {
		if e := recover(); e != nil {
			out = "panic"
		}
	}()
	var m amqp.Message
	if err := m.UnmarshalBinary(b); err != nil {
		if fzWhy {
			fmt.Fprintf(os.Stderr, "case %d: %v\n", fzIdx, err)
		}
		// what go-amqp cannot decode and std/amqp can (its documented superset): char and the
		// decimals, a described value where go-amqp wants a plain one, a list, map or binary
		// map key, a typed field in another integer encoding than go-amqp's
		e := err.Error()
		for _, x := range []string{"not implemented", "invalid type for", "invalid map key"} {
			if strings.Contains(e, x) {
				return "unsupported"
			}
		}
		return "error"
	}
	if m.Header != nil && fzTTLPastDuration(b) {
		return "unsupported"
	}
	fzBadUTF8 = false
	func() {
		defer func() {
			if e := recover(); e != nil {
				out = "unshowable"
			}
		}()
		out = showMsg(&m)
	}()
	if out != "unshowable" && (fzRefused(b) || fzDroppedBadUTF8(b)) {
		return "error"
	}
	return out
}

func fuzzMain(seed int64, n int) {
	r := rand.New(rand.NewSource(seed))
	var w strings.Builder
	for k := 0; k < n; k++ {
		b, err := fzMessage(r).MarshalBinary()
		if err != nil {
			panic(fmt.Sprintf("marshal: %v", err))
		}
		if r.Intn(5) != 0 {
			b = fzMutate(r, b)
		}
		fzIdx = k
		fmt.Fprintf(&w, "== \nc %s\nw %s\n", hex.EncodeToString(b), fzDecode(b))
	}
	fmt.Print(w.String())
}
