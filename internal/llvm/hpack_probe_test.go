package llvm

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// hpackHuff is the Huffman code of s (RFC 7541 5.2, padded with ones).
func hpackHuff(s string) []byte {
	var out []byte
	var acc uint64
	bits := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		acc = acc<<hpackHuffLen[c] | uint64(hpackHuffCode[c])
		bits += int(hpackHuffLen[c])
		for bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>bits))
		}
		acc &= 1<<bits - 1
	}
	if bits > 0 {
		out = append(out, byte(acc<<(8-bits))|byte(0xff>>bits))
	}
	return out
}

// hpackStr appends a string literal, Huffman coded or raw.
func hpackStr(b []byte, s string, huff bool) []byte {
	if huff {
		h := hpackHuff(s)
		return append(hpackInt(b, 0x80, 7, len(h)), h...)
	}
	return append(hpackInt(b, 0, 7, len(s)), s...)
}

// hpackTrace decodes the server's header blocks for probeH2HPACK and
// shows each representation: `i<n>` indexed, `u<n>` a table size update,
// a literal as L (with indexing), N (without), or V (never indexed) with
// its name's index or its name, and its value, H before a Huffman coded
// string and R before a raw one; the value of date is X. Equal traces
// are equal blocks but for the date.
type hpackTrace struct {
	dyn       [][2]string // the newest first
	size, max int
}

func (d *hpackTrace) huff(b []byte) string {
	var out []byte
	var acc uint64
	bits := 0
	for _, x := range b {
		acc = acc<<8 | uint64(x)
		bits += 8
		for {
			found := false
			for c := 0; c < 256; c++ {
				n := int(hpackHuffLen[c])
				if n <= bits && uint32(acc>>(bits-n))&(1<<n-1) == hpackHuffCode[c] {
					out = append(out, byte(c))
					bits -= n
					acc &= 1<<bits - 1
					found = true
					break
				}
			}
			if !found {
				break
			}
		}
	}
	return string(out)
}

func (d *hpackTrace) evict() {
	for d.size > d.max && len(d.dyn) > 0 {
		e := d.dyn[len(d.dyn)-1]
		d.size -= len(e[0]) + len(e[1]) + 32
		d.dyn = d.dyn[:len(d.dyn)-1]
	}
}

func (d *hpackTrace) block(b []byte) string {
	var out []string
	i := 0
	getInt := func(prefix int) int {
		max := 1<<prefix - 1
		v := int(b[i]) & max
		i++
		if v < max {
			return v
		}
		m := 0
		for i < len(b) {
			c := b[i]
			i++
			v += int(c&127) << m
			m += 7
			if c&128 == 0 {
				break
			}
		}
		return v
	}
	getStr := func() (string, string) {
		if i >= len(b) {
			return "", "?"
		}
		huff := b[i]&0x80 != 0
		n := getInt(7)
		if i+n > len(b) {
			n = len(b) - i
		}
		raw := b[i : i+n]
		i += n
		if huff {
			return d.huff(raw), "H"
		}
		return string(raw), "R"
	}
	for i < len(b) {
		c := b[i]
		switch {
		case c&0x80 != 0:
			out = append(out, fmt.Sprintf("i%d", getInt(7)))
		case c&0xe0 == 0x20:
			v := getInt(5)
			d.max = v
			d.evict()
			out = append(out, fmt.Sprintf("u%d", v))
		default:
			kind, prefix := "N", 4
			if c&0xc0 == 0x40 {
				kind, prefix = "L", 6
			} else if c&0xf0 == 0x10 {
				kind = "V"
			}
			idx := getInt(prefix)
			name, nk := "", ""
			switch {
			case idx == 0:
				name, nk = getStr()
				nk += fmt.Sprintf("%q", name)
			case idx <= 61:
				name, nk = hpackStatic[idx-1][0], fmt.Sprint(idx)
			case idx-62 < len(d.dyn):
				name, nk = d.dyn[idx-62][0], fmt.Sprint(idx)
			default:
				name, nk = "?", fmt.Sprint(idx)
			}
			value, vk := getStr()
			shown := value
			if name == "date" {
				shown = "X"
			}
			out = append(out, fmt.Sprintf("%s%s=%s%q", kind, nk, vk, shown))
			if kind == "L" {
				d.dyn = append([][2]string{{name, value}}, d.dyn...)
				d.size += len(name) + len(value) + 32
				d.evict()
			}
		}
	}
	return strings.Join(out, " ")
}

// h2Seq runs the steps on h one at a time: a step sends frames and
// returns the stream whose end it waits for (0: none). It shows each
// stream's header blocks traced (hpackTrace, one decoder for the
// connection) and its body.
func h2Seq(h *h2conn, steps []func(h *h2conn) uint32) string {
	defer h.c.Close()
	d := &hpackTrace{max: 4096}
	var res []string
	for _, step := range steps {
		sid := step(h)
		if sid == 0 {
			continue
		}
		var evs []string
		var body []byte
		h.c.SetReadDeadline(time.Now().Add(h2EventsMax))
		for done := false; !done; {
			typ, flags, fsid, p, err := h.read()
			if err != nil {
				evs = append(evs, "closed")
				res = append(res, strings.Join(evs, " "))
				return strings.Join(res, "; ")
			}
			switch typ {
			case 0:
				body = append(body, p...)
				done = flags&1 != 0 && fsid == sid
			case 1, 9:
				blk := p
				if typ == 1 && flags&8 != 0 {
					blk = blk[1 : len(blk)-int(p[0])]
				}
				if typ == 1 && flags&0x20 != 0 {
					blk = blk[5:]
				}
				evs = append(evs, fmt.Sprintf("%d[%s]", fsid, d.block(blk)))
				done = flags&1 != 0 && fsid == sid
			case 3:
				evs = append(evs, "RST_STREAM "+h2Code(binary.BigEndian.Uint32(p)))
				done = fsid == sid
			case 7:
				evs = append(evs, "GOAWAY "+h2Code(binary.BigEndian.Uint32(p[4:])))
				done = true
			}
		}
		if len(body) > 0 {
			evs = append(evs, fmt.Sprintf("%q", body))
		}
		res = append(res, strings.Join(evs, " "))
	}
	return strings.Join(res, "; ")
}

// probeH2HPACK checks the server's HPACK (decision 0193 step 6) against
// the Go backend's: RFC 7541's request examples (C.3 raw, C.4 Huffman
// coded, and C.4 with each block cut across a CONTINUATION), one
// connection each, the decoded fields echoed in the body; the dynamic
// table filled past its size and read back; every allowed byte Huffman
// coded; the encoder's blocks over several responses, traced
// representation by representation, with the table sizes the client
// sets; and decoding errors (Huffman padding and EOS, integer overflow,
// table size updates).
func probeH2HPACK(out *strings.Builder, addr string, p *testPKI) {
	hexb := func(s string) []byte {
		b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
		if err != nil {
			panic(err)
		}
		return b
	}
	c3 := [][]byte{
		hexb("8286 8441 0f77 7777 2e65 7861 6d70 6c65 2e63 6f6d"),
		hexb("8286 84be 5808 6e6f 2d63 6163 6865"),
		hexb("8287 85bf 400a 6375 7374 6f6d 2d6b 6579 0c63 7573 746f 6d2d 7661 6c75 65"),
	}
	c4 := [][]byte{
		hexb("8286 8441 8cf1 e3c2 e5f2 3a6b a0ab 90f4 ff"),
		hexb("8286 84be 5886 a8eb 1064 9cbf"),
		hexb("8287 85bf 4088 25a8 49e9 5ba9 7d7f 8925 a849 e95b b8e8 b4bf"),
	}
	blocks := func(bs [][]byte, cut bool) []func(h *h2conn) uint32 {
		var steps []func(h *h2conn) uint32
		for k, b := range bs {
			sid := uint32(2*k + 1)
			steps = append(steps, func(h *h2conn) uint32 {
				if cut {
					m := len(b) / 2
					h.frame(1, 1, sid, b[:m])
					h.frame(9, 4, sid, b[m:])
				} else {
					h.frame(1, 5, sid, b)
				}
				return sid
			})
		}
		return steps
	}
	block := func(sid uint32, b []byte) func(h *h2conn) uint32 {
		return func(h *h2conn) uint32 {
			h.frame(1, 5, sid, b)
			return sid
		}
	}
	req := func(sid uint32, path string) func(h *h2conn) uint32 {
		return func(h *h2conn) uint32 {
			h.headers(sid, true, h2Req("GET", path)...)
			return sid
		}
	}
	tableSize := func(vs ...uint32) func(h *h2conn) uint32 {
		return func(h *h2conn) uint32 {
			for _, v := range vs {
				b := make([]byte, 6)
				binary.BigEndian.PutUint16(b, 1)
				binary.BigEndian.PutUint32(b[2:], v)
				h.frame(4, 0, 0, b)
			}
			return 0
		}
	}
	// GET https://x/ and then the fields
	get := func(fields ...byte) []byte {
		b := []byte{0x82, 0x87, 0x84, 0x41}
		b = hpackStr(b, "x", false)
		return append(b, fields...)
	}
	// 60 fields with indexing (over 7 KB: the 4096 byte table evicts),
	// then the newest 20 by index
	var fill []byte
	for k := 0; k < 60; k++ {
		fill = append(fill, 0x40)
		fill = hpackStr(fill, fmt.Sprintf("x-f%02d", k), k%2 == 0)
		fill = hpackStr(fill, strings.Repeat(string(rune('a'+k%26)), 60+k), k%3 == 0)
	}
	var back []byte
	for k := 62; k < 62+20; k++ {
		back = hpackInt(back, 0x80, 7, k)
	}
	var every strings.Builder
	for c := 0x20; c < 0x100; c++ {
		if c != 0x7f {
			every.WriteByte(byte(c))
		}
	}
	var all []byte
	all = append(all, 0x00)
	all = hpackStr(all, "x-all", true)
	all = hpackStr(all, every.String(), true)
	all = append(all, 0x10)
	all = hpackStr(all, "x-never", false)
	all = hpackStr(all, "secret", true)
	type seqCase struct {
		name  string
		steps []func(h *h2conn) uint32
	}
	seqs := []seqCase{
		{"rfc7541 c.3", blocks(c3, false)},
		{"rfc7541 c.4", blocks(c4, false)},
		{"rfc7541 c.4 cut", blocks(c4, true)},
		{"table filled and read back", []func(h *h2conn) uint32{block(1, get(fill...)), block(3, get(back...))}},
		{"every byte huffman coded", []func(h *h2conn) uint32{block(1, get(all...))}},
		{"encoder", []func(h *h2conn) uint32{req(1, "/fields"), req(3, "/hello"), req(5, "/fields"), req(7, "/json"), req(9, "/fields")}},
		{"encoder table size 0", []func(h *h2conn) uint32{tableSize(0), req(1, "/fields"), req(3, "/fields")}},
		{"encoder table size 50 then 300", []func(h *h2conn) uint32{req(1, "/fields"), tableSize(50, 300), req(3, "/fields"), req(5, "/fields")}},
		{"encoder table size over 4096", []func(h *h2conn) uint32{tableSize(100000), req(1, "/fields"), req(3, "/fields")}},
		{"100 continue", []func(h *h2conn) uint32{func(h *h2conn) uint32 {
			h.headers(1, false, h2Req("POST", "/echo", "expect", "100-continue")...)
			time.Sleep(100 * time.Millisecond)
			h.frame(0, 1, 1, []byte("zz"))
			return 1
		}, req(3, "/fields")}},
	}
	res := make([]string, len(seqs))
	var wg sync.WaitGroup
	for i, sc := range seqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := h2Dial(addr, p)
			if err != nil {
				res[i] = err.Error()
				return
			}
			res[i] = h2Seq(h, sc.steps)
		}()
	}
	wg.Wait()
	for i, sc := range seqs {
		fmt.Fprintf(out, "hpack %s: %s\n", sc.name, res[i])
	}
	// decoding errors, each on a connection of its own (events)
	huffA := hpackHuff("a") // 00011 and the padding 111
	lit := func(code []byte) []byte { return append(hpackInt(nil, 0x80, 7, len(code)), code...) }
	value := func(v []byte) []byte {
		b := append([]byte{0x00}, hpackStr(nil, "x-v", false)...)
		return get(append(b, v...)...)
	}
	raws := []struct {
		name string
		blk  []byte
	}{
		{"huffman padding of 8 bits", value(lit(append(append([]byte{}, huffA...), 0xff)))},
		{"huffman padding with a zero", value(lit([]byte{huffA[0] &^ 1}))},
		{"huffman eos", value(lit([]byte{0xff, 0xff, 0xff, 0xff}))},
		{"huffman empty", value(lit(nil))},
		{"huffman 7 bits of padding", value(lit(hpackHuff("0aZ")))},
		{"huffman long codes", value(lit(hpackHuff("\xfe\xff\x80\xc0")))},
		{"index overflow", get(append([]byte{0xff}, bytes.Repeat([]byte{0xff}, 10)...)...)},
		{"index of 8 continuation bytes", get(append(append([]byte{0xff}, bytes.Repeat([]byte{0x80}, 8)...), 0x01)...)},
		{"string length past the block", value([]byte{0x7f, 0x80, 0x01})},
		{"string length of 2^63", value(append(append([]byte{0x7f}, bytes.Repeat([]byte{0xff}, 8)...), 0x7f))},
		{"size update to 0 first", append([]byte{0x20}, value(hpackStr(nil, "v", false))...)},
		{"two size updates first", append([]byte{0x20, 0x3f, 0xe1, 0x1f}, value(hpackStr(nil, "v", false))...)},
		{"size update after a field, empty table", append([]byte{0x82, 0x20}, value(hpackStr(nil, "v", false))[1:]...)},
		{"literal name index past the table", get(0x4f, 0x30, 0x01, 'v')},
		{"never indexed literal", value(append(hpackStr(nil, "v", false), append([]byte{0x10}, append(hpackStr(nil, "x-n", true), hpackStr(nil, "w", true)...)...)...))},
	}
	rres := make([]string, len(raws))
	for i, rc := range raws {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := h2Dial(addr, p)
			if err != nil {
				rres[i] = err.Error()
				return
			}
			h.frame(1, 5, 1, rc.blk)
			rres[i] = h.events(1500 * time.Millisecond)
		}()
	}
	wg.Wait()
	for i, rc := range raws {
		fmt.Fprintf(out, "hpack %s: %s\n", rc.name, rres[i])
	}
}
