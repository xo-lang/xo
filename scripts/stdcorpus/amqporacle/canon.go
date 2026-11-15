package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

// canon gives an encoding whose maps have their entries sorted by the
// keys' encoded bytes. go-amqp writes a Go map in Go's iteration order,
// which differs from run to run, so the corpus would change at every
// regeneration (stdcorpus -check reported amqp stale on every machine,
// 2026-11-15); a map's entries may come in any order, and std/amqp
// encodes a decoded map back in the order it read. Only the order
// changes: every length stays.
func canon(b []byte) []byte {
	var out []byte
	for len(b) > 0 {
		v, rest := canonValue(b)
		out = append(out, v...)
		b = rest
	}
	return out
}

func canonValue(b []byte) ([]byte, []byte) {
	c := b[0]
	if c == 0x00 {
		d, r := canonValue(b[1:])
		v, r2 := canonValue(r)
		return append(append([]byte{0x00}, d...), v...), r2
	}
	body, rest := canonBody(c, b[1:])
	return append([]byte{c}, body...), rest
}

func width(c byte) int {
	if c>>4 == 0xc || c>>4 == 0xe {
		return 1
	}
	return 4
}

func size(b []byte, w int) int {
	if w == 1 {
		return int(b[0])
	}
	return int(binary.BigEndian.Uint32(b))
}

// canonBody reads the payload of a value of constructor c from b.
func canonBody(c byte, b []byte) ([]byte, []byte) {
	switch c >> 4 {
	case 0x4:
		return nil, b
	case 0x5:
		return b[:1], b[1:]
	case 0x6:
		return b[:2], b[2:]
	case 0x7:
		return b[:4], b[4:]
	case 0x8:
		return b[:8], b[8:]
	case 0x9:
		return b[:16], b[16:]
	case 0xa:
		n := int(b[0])
		return b[:1+n], b[1+n:]
	case 0xb:
		n := size(b, 4)
		return b[:4+n], b[4+n:]
	case 0xc, 0xd:
		w := width(c)
		n := size(b, w)
		head, content, rest := b[:2*w], b[w : w+n][w:], b[w+n:]
		count := size(b[w:], w)
		var items [][]byte
		for i := 0; i < count; i++ {
			v, r := canonValue(content)
			items = append(items, v)
			content = r
		}
		if len(content) != 0 {
			panic(fmt.Sprintf("canon: %d bytes after a compound's items", len(content)))
		}
		if c == 0xc1 || c == 0xd1 {
			type pair struct{ k, v []byte }
			var ps []pair
			for i := 0; i+1 < len(items); i += 2 {
				ps = append(ps, pair{items[i], items[i+1]})
			}
			sort.SliceStable(ps, func(i, j int) bool { return bytes.Compare(ps[i].k, ps[j].k) < 0 })
			items = items[:0]
			for _, p := range ps {
				items = append(items, p.k, p.v)
			}
		}
		out := append([]byte{}, head...)
		for _, it := range items {
			out = append(out, it...)
		}
		return out, rest
	case 0xe, 0xf:
		w := width(c)
		n := size(b, w)
		head, content, rest := b[:2*w], b[w : w+n][w:], b[w+n:]
		count := size(b[w:], w)
		out := append([]byte{}, head...)
		ec := content[0]
		out = append(out, ec)
		content = content[1:]
		if ec == 0x00 {
			d, r := canonValue(content)
			out = append(out, d...)
			ec = r[0]
			out = append(out, ec)
			content = r[1:]
		}
		for i := 0; i < count; i++ {
			v, r := canonBody(ec, content)
			out = append(out, v...)
			content = r
		}
		if len(content) != 0 {
			panic(fmt.Sprintf("canon: %d bytes after an array's elements", len(content)))
		}
		return out, rest
	}
	panic(fmt.Sprintf("canon: unknown constructor %#x", c))
}
