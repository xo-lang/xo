package main

import (
	"crypto/subtle"
	"fmt"
	"math"
	mrand "math/rand"
	"strings"
)

// std/crypto's ct_byte_eq, ct_eq, ct_select, ct_copy and ct_less_or_eq
// against Go's crypto/subtle: constant_time_test.go's
// TestConstantTimeByteEq, TestConstantTimeEq, TestConstantTimeCopy and
// lessOrEqTests (Copyright 2009 The Go Authors, BSD-style license), and
// seeded values over Go's domains (a byte, int32, 0 to 2^31 - 1). Past
// them, where Xo's take 64 bits, the rows are x == y and x <= y.

func subtleCorpus() string {
	var o strings.Builder
	o.WriteString(header("Go's crypto/subtle and its constant_time_test.go (Copyright 2009 The Go Authors, BSD-style license)"))
	o.WriteString("\nuse std/crypto\n")
	r := mrand.New(mrand.NewSource(2009))
	edges := []int64{0, 1, -1, 2, 127, 128, 255, 256, 257, math.MaxInt32, math.MinInt32, math.MaxInt32 + 1, math.MaxInt64, math.MinInt64, 1 << 32, -256}

	o.WriteString("\n/// (x, y, Go's ConstantTimeByteEq of their low bytes)\nconst BYTE_EQ: List[(Int, Int, Int)] = [\n")
	var pairs [][2]int64
	for _, x := range edges {
		for _, y := range edges {
			pairs = append(pairs, [2]int64{x, y})
		}
	}
	for i := 0; i < 300; i++ {
		x := int64(r.Intn(256))
		y := int64(r.Intn(256))
		if i%3 == 0 {
			y = x
		}
		if i%5 == 0 {
			y = x + 256*int64(r.Intn(1000)-500)
		}
		pairs = append(pairs, [2]int64{x, y})
	}
	for _, p := range pairs {
		fmt.Fprintf(&o, "  (%d, %d, %d),\n", p[0], p[1], subtle.ConstantTimeByteEq(uint8(p[0]), uint8(p[1])))
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (x, y, Go's ConstantTimeEq when both are int32, else x == y)\nconst EQ: List[(Int, Int, Int)] = [\n")
	pairs = pairs[:0]
	for _, x := range edges {
		for _, y := range edges {
			pairs = append(pairs, [2]int64{x, y})
		}
	}
	for i := 0; i < 300; i++ {
		x := int64(int32(r.Uint32()))
		if i%2 == 0 {
			x = int64(r.Uint64())
		}
		y := x
		switch i % 4 {
		case 1:
			y = x ^ (1 << uint(r.Intn(64)))
		case 2:
			y = int64(int32(r.Uint32()))
		}
		pairs = append(pairs, [2]int64{x, y})
	}
	for _, p := range pairs {
		want := 0
		if p[0] == p[1] {
			want = 1
		}
		if p[0] == int64(int32(p[0])) && p[1] == int64(int32(p[1])) {
			want = subtle.ConstantTimeEq(int32(p[0]), int32(p[1]))
		}
		fmt.Fprintf(&o, "  (%d, %d, %d),\n", p[0], p[1], want)
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (v, x, y, Go's ConstantTimeSelect of v's low bit)\nconst SELECT: List[(Int, Int, Int, Int)] = [\n")
	for i := 0; i < 200; i++ {
		v := int64(i % 2)
		if i%7 == 0 {
			v = int64(r.Intn(1000) - 500)
		}
		x, y := int64(r.Uint64()), int64(r.Uint64())
		if i < len(edges) {
			x, y = edges[i], edges[len(edges)-1-i]
		}
		fmt.Fprintf(&o, "  (%d, %d, %d, %d),\n", v, x, y, subtle.ConstantTimeSelect(int(v&1), int(x), int(y)))
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (v, x, y, Go's ConstantTimeCopy of y into x under v's low bit)\nconst COPY: List[(Int, Bytes, Bytes, Bytes)] = [\n")
	for _, n := range []int{0, 1, 2, 7, 8, 9, 16, 31, 33, 100} {
		for _, v := range []int64{0, 1, 2, 3, -1} {
			x, y := make([]byte, n), make([]byte, n)
			r.Read(x)
			r.Read(y)
			d := append([]byte{}, x...)
			subtle.ConstantTimeCopy(int(v&1), d, y)
			fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", v, xbytes(x), xbytes(y), xbytes(d))
		}
	}
	o.WriteString("]\n")

	o.WriteString("\n/// (x, y, Go's ConstantTimeLessOrEq when both are below 2^31, else x <= y): lessOrEqTests first\nconst LESS_OR_EQ: List[(Int, Int, Int)] = [\n")
	pairs = [][2]int64{{0, 0}, {1, 0}, {0, 1}, {10, 20}, {20, 10}, {10, 10}}
	nonneg := []int64{0, 1, 2, 255, 256, math.MaxInt32 - 1, math.MaxInt32, math.MaxInt32 + 1, 1 << 32, math.MaxInt64 - 1, math.MaxInt64}
	for _, x := range nonneg {
		for _, y := range nonneg {
			pairs = append(pairs, [2]int64{x, y})
		}
	}
	for i := 0; i < 300; i++ {
		x, y := int64(r.Int31()), int64(r.Int31())
		if i%2 == 0 {
			x, y = r.Int63(), r.Int63()
		}
		if i%5 == 0 {
			y = x + int64(r.Intn(3)-1)
			if y < 0 {
				y = 0
			}
		}
		pairs = append(pairs, [2]int64{x, y})
	}
	for _, p := range pairs {
		want := 0
		if p[0] <= p[1] {
			want = 1
		}
		if p[0] <= math.MaxInt32 && p[1] <= math.MaxInt32 {
			want = subtle.ConstantTimeLessOrEq(int(p[0]), int(p[1]))
		}
		fmt.Fprintf(&o, "  (%d, %d, %d),\n", p[0], p[1], want)
	}
	o.WriteString("]\n")

	o.WriteString(`
fn show(bad: List[Str]) -> List[Str] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

test "ct_byte_eq is Go's ConstantTimeByteEq of the low bytes (TestConstantTimeByteEq)" {
  var bad: List[Str] = []
  for (x, y, want) in BYTE_EQ {
    if crypto.ct_byte_eq(x, y) != want {
      bad.push("${x} ${y}")
    }
  }
  expect(show(bad)) == []
}

test "ct_eq is Go's ConstantTimeEq, over 64 bits (TestConstantTimeEq)" {
  var bad: List[Str] = []
  for (x, y, want) in EQ {
    if crypto.ct_eq(x, y) != want {
      bad.push("${x} ${y}")
    }
  }
  expect(show(bad)) == []
}

test "ct_select is Go's ConstantTimeSelect" {
  var bad: List[Str] = []
  for (v, x, y, want) in SELECT {
    if crypto.ct_select(v, x, y) != want {
      bad.push("${v} ${x} ${y}")
    }
  }
  expect(show(bad)) == []
}

test "ct_copy is Go's ConstantTimeCopy into a new Bytes (TestConstantTimeCopy)" {
  var bad: List[Str] = []
  for (v, x, y, want) in COPY {
    if crypto.ct_copy(v, x, y) != want {
      bad.push("${v} ${x.len()}")
    }
  }
  expect(show(bad)) == []
}

test "ct_less_or_eq is Go's ConstantTimeLessOrEq, over 64 bits (TestConstantTimeLessOrEq)" {
  var bad: List[Str] = []
  for (x, y, want) in LESS_OR_EQ {
    if crypto.ct_less_or_eq(x, y) != want {
      bad.push("${x} ${y}")
    }
  }
  expect(show(bad)) == []
}
`)
	return o.String()
}
