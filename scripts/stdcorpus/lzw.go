package main

import (
	"bytes"
	"compress/lzw"
	"fmt"
	"io"
	"math/rand"
	"strings"
)

// lzwCorpus is compress/lzw: the writer over seeded data of every literal
// width and both orders (including inputs long enough to run the 12 bit
// codes out and reset the table), the reader over those streams, over
// truncations and over random bytes (Go's output before its error, and its
// error text), and the argument errors.
func lzwCorpus() string {
	var o strings.Builder
	o.WriteString(header("compress/lzw (std/compress lzw_encode and lzw_decode, decision 0578)"))
	o.WriteString("use std/compress\n\n")
	r := rand.New(rand.NewSource(578))
	lit := func(b []byte) string {
		var sb strings.Builder
		sb.WriteString(`b"`)
		for _, c := range b {
			fmt.Fprintf(&sb, `\x%02x`, c)
		}
		sb.WriteString(`"`)
		return sb.String()
	}
	enc := func(data []byte, order lzw.Order, lw int) ([]byte, string) {
		var buf bytes.Buffer
		w := lzw.NewWriter(&buf, order, lw)
		if _, err := w.Write(data); err != nil {
			return nil, err.Error()
		}
		if err := w.Close(); err != nil {
			return nil, err.Error()
		}
		return buf.Bytes(), ""
	}
	dec := func(data []byte, order lzw.Order, lw int) ([]byte, string) {
		rd := lzw.NewReader(bytes.NewReader(data), order, lw)
		out, err := io.ReadAll(rd)
		if err != nil {
			return out, err.Error()
		}
		return out, ""
	}
	gen := func(n, k, lw int) []byte {
		b := make([]byte, n)
		maxv := 1 << lw
		if k > maxv {
			k = maxv
		}
		for i := range b {
			if r.Intn(3) == 0 && i > 8 {
				b[i] = b[i-1-r.Intn(8)]
			} else {
				b[i] = byte(r.Intn(k))
			}
		}
		return b
	}
	o.WriteString("/// (order 0 LSB or 1 MSB, lit width, data, Go's encoding or \"\", Go's error text)\nconst ENC_CASES: List[(Int, Int, Bytes, Bytes, Str)] = [\n")
	type dcase struct {
		order int
		lw    int
		data  []byte
	}
	var decs []dcase
	for order := 0; order < 2; order++ {
		for lw := 2; lw <= 8; lw++ {
			for _, n := range []int{0, 1, 2, 7, 100, 1500, 30000} {
				for _, k := range []int{2, 5, 256} {
					if n > 1500 && k != 256 && k != 5 {
						continue
					}
					data := gen(n, k, lw)
					out, e := enc(data, lzw.Order(order), lw)
					fmt.Fprintf(&o, "  (%d, %d, %s, %s, %q),\n", order, lw, lit(data), lit(out), e)
					if e == "" {
						decs = append(decs, dcase{order, lw, out})
					}
				}
			}
		}
	}
	// a byte too large for the literal width, and bad widths
	for _, lw := range []int{1, 2, 9, -3} {
		out, e := enc([]byte{0, 1, 5}, lzw.LSB, lw)
		fmt.Fprintf(&o, "  (0, %d, %s, %s, %q),\n", lw, lit([]byte{0, 1, 5}), lit(out), e)
	}
	o.WriteString("]\n")
	// readers: valid streams, truncations, corruptions, random bytes
	o.WriteString("\n/// (order, lit width, stream, Go's output, Go's error text)\nconst DEC_CASES: List[(Int, Int, Bytes, Bytes, Str)] = [\n")
	n := 0
	for _, d := range decs {
		if n%3 == 0 || len(d.data) < 40 {
			out, e := dec(d.data, lzw.Order(d.order), d.lw)
			fmt.Fprintf(&o, "  (%d, %d, %s, %s, %q),\n", d.order, d.lw, lit(d.data), lit(out), e)
		}
		n++
		if len(d.data) > 4 && n%2 == 0 {
			cut := d.data[:len(d.data)/2]
			out, e := dec(cut, lzw.Order(d.order), d.lw)
			fmt.Fprintf(&o, "  (%d, %d, %s, %s, %q),\n", d.order, d.lw, lit(cut), lit(out), e)
			bad := append([]byte(nil), d.data...)
			bad[r.Intn(len(bad))] ^= byte(1 << r.Intn(8))
			out, e = dec(bad, lzw.Order(d.order), d.lw)
			fmt.Fprintf(&o, "  (%d, %d, %s, %s, %q),\n", d.order, d.lw, lit(bad), lit(out), e)
		}
	}
	for i := 0; i < 120; i++ {
		order, lw := r.Intn(2), 2+r.Intn(7)
		b := make([]byte, 1+r.Intn(60))
		r.Read(b)
		out, e := dec(b, lzw.Order(order), lw)
		fmt.Fprintf(&o, "  (%d, %d, %s, %s, %q),\n", order, lw, lit(b), lit(out), e)
	}
	for _, lw := range []int{0, 1, 9} {
		out, e := dec([]byte{1, 2, 3}, lzw.LSB, lw)
		fmt.Fprintf(&o, "  (0, %d, %s, %s, %q),\n", lw, lit([]byte{1, 2, 3}), lit(out), e)
	}
	o.WriteString("]\n")
	o.WriteString(`
fn order_of(i: Int) -> compress.LzwOrder {
  if i == 0 { compress.LzwOrder.LSB } else { compress.LzwOrder.MSB }
}

test "lzw_encode is Go's compress/lzw writer" {
  for (order, lw, data, want, err) in ENC_CASES {
    match compress.lzw_encode(data, order_of(order), lw) {
      Ok(got) => {
        expect((order, lw, data.len(), "", got)) == (order, lw, data.len(), err, want)
      }
      Err(e) => {
        expect((order, lw, data.len(), e.msg)) == (order, lw, data.len(), err)
      }
    }
  }
}

test "lzw_decode round trips and matches Go's reader" {
  for (order, lw, data, want, err) in ENC_CASES {
    if err == "" {
      match compress.lzw_decode(want, order_of(order), lw) {
        Ok(got) => {
          expect((order, lw, got)) == (order, lw, data)
        }
        Err(e) => {
          expect((order, lw, e.msg)) == (order, lw, "")
        }
      }
    }
  }
  for (order, lw, data, want, err) in DEC_CASES {
    match compress.lzw_decode(data, order_of(order), lw) {
      Ok(got) => {
        expect((order, lw, data.len(), "", got)) == (order, lw, data.len(), err, want)
      }
      Err(e) => {
        expect((order, lw, data.len(), e.msg)) == (order, lw, data.len(), err)
      }
    }
  }
}

test "lzw_decode stops at max_size" {
  match compress.lzw_decode(b"\x04\x01", compress.LzwOrder.LSB, 2, 0) {
    Ok(_) => expect(false) == true
    Err(e) => expect(e.msg) == "lzw: output exceeds max_size (0 bytes)"
  }
}
`)
	return o.String()
}
