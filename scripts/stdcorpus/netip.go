package main

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"net/netip"
	"strconv"
	"strings"
)

// netipAddrText is the corpus's text of a parsed Addr: BitLen, As16, Zone,
// String, the predicates, Unmap's String, AsSlice.
func netipAddrText(a netip.Addr) string {
	ip := a.As16()
	b := func(x bool) string {
		if x {
			return "1"
		}
		return "0"
	}
	return fmt.Sprintf("%d|%s|%s|%s|%s%s%s%s%s%s%s%s|%s|%s", a.BitLen(), hex.EncodeToString(ip[:]), textOf(a.Zone()), textOf(a.String()),
		b(a.Is4()), b(a.Is6()), b(a.Is4In6()), b(a.IsLoopback()), b(a.IsPrivate()), b(a.IsUnspecified()), b(a.IsLinkLocalUnicast()), b(a.IsMulticast()),
		textOf(a.Unmap().String()), hex.EncodeToString(a.AsSlice()))
}

func netipV4(r *rand.Rand) string {
	parts := make([]string, 4)
	for i := range parts {
		switch r.Intn(10) {
		case 0:
			parts[i] = []string{"256", "01", "00", "", "1a", "-1", "999"}[r.Intn(7)]
		case 1:
			parts[i] = []string{"0", "255", "127", "10", "172", "192", "169", "254", "224", "100"}[r.Intn(10)]
		default:
			parts[i] = strconv.Itoa(r.Intn(256))
		}
	}
	if r.Intn(12) == 0 {
		return strings.Join(parts[:1+r.Intn(3)], ".")
	}
	return strings.Join(parts, ".")
}

func netipV6(r *rand.Rand) string {
	n := 8
	if r.Intn(3) == 0 {
		n = 6
	}
	gs := make([]string, n)
	for i := range gs {
		switch r.Intn(8) {
		case 0:
			gs[i] = "0"
		case 1:
			gs[i] = []string{"fe80", "ff02", "fc00", "fd12", "2001", "db8", "ffff", "FFFF", "00000", "g", ""}[r.Intn(11)]
		default:
			gs[i] = strconv.FormatInt(int64(r.Intn(65536)), 16)
		}
	}
	s := strings.Join(gs, ":")
	if n == 6 {
		s += ":" + netipV4(r)
	}
	if r.Intn(3) == 0 {
		// compress a run
		i := r.Intn(len(gs))
		j := i + r.Intn(len(gs)-i)
		s = strings.Join(gs[:i], ":") + "::" + strings.Join(gs[j:], ":")
		if n == 6 && r.Intn(2) == 0 {
			s += ":" + netipV4(r)
		}
	}
	if r.Intn(5) == 0 {
		s = []string{"::", "::1", "::ffff:" + netipV4(r), "fe80::1", "64:ff9b::" + netipV4(r), ":::", "1::2::3", "::ffff:1.2.3"}[r.Intn(8)]
	}
	if r.Intn(6) == 0 {
		s += "%" + []string{"eth0", "1", "", "en%1", "é"}[r.Intn(5)]
	}
	return s
}

func netipCorpus() string {
	r := rand.New(rand.NewSource(7))
	var o strings.Builder
	o.WriteString(header("net/netip's ParseAddr, ParsePrefix, AddrFromSlice, and Compare as std/rt/netip.go calls them, on seeded random IPv4 and IPv6 texts (compressed runs, embedded IPv4, zones, bad parts)"))
	o.WriteString("use std/netip\n\n/// (text, Go's result: BITS|AS16|ZONE|STRING|is_4 is_6 is_4in6 loopback private unspecified link_local multicast|UNMAP|BYTES, or err TEXT)\nconst ADDRS: List[(Str, Str)] = [\n")
	var good []netip.Addr
	for i := 0; i < 2500; i++ {
		var s string
		if r.Intn(2) == 0 {
			s = netipV4(r)
		} else {
			s = netipV6(r)
		}
		s = textOf(s)
		a, err := netip.ParseAddr(s)
		w := ""
		if err != nil {
			w = "err " + textOf(err.Error())
		} else {
			w = netipAddrText(a)
			good = append(good, a)
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(w))
	}
	o.WriteString("]\n\n/// (text, Go's result: STRING|MASKED|contains of each probe, or err TEXT)\nconst PREFIXES: List[(Str, Str)] = [\n")
	for i := 0; i < 1200; i++ {
		// a valid address of each family most of the time, then a length
		a := good[r.Intn(len(good))].String()
		if r.Intn(4) == 0 {
			if r.Intn(2) == 0 {
				a = netipV4(r)
			} else {
				a = netipV6(r)
			}
		}
		max := 32
		if strings.Contains(a, ":") {
			max = 128
		}
		s := textOf(a + "/" + []string{strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(max + 1)), strconv.Itoa(r.Intn(129)), "", "-1", "08", "x"}[r.Intn(11)])
		p, err := netip.ParsePrefix(s)
		w := ""
		if err != nil {
			w = "err " + textOf(err.Error())
		} else {
			var cs []byte
			for k := 0; k < 8; k++ {
				pr := good[(i*8+k)%len(good)]
				if p.Contains(pr) {
					cs = append(cs, '1')
				} else {
					cs = append(cs, '0')
				}
			}
			w = textOf(p.String()) + "|" + textOf(p.Masked().String()) + "|" + string(cs)
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(s), xq(w))
	}
	o.WriteString("]\n\n/// (bytes, Go's AddrFromSlice: STRING or none)\nconst SLICES: List[(Bytes, Str)] = [\n")
	for i := 0; i < 200; i++ {
		b := make([]byte, []int{0, 4, 16, 5, 15, 4, 16}[r.Intn(7)])
		r.Read(b)
		w := "none"
		if a, ok := netip.AddrFromSlice(b); ok {
			w = a.String()
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xbytes(b), xq(w))
	}
	o.WriteString("]\n")
	o.WriteString(netipTests)
	return o.String()
}

const netipTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn bit(x: Bool) -> Str { if x { "1" } else { "0" } }

fn addr_text(a: netip.Addr) -> Str {
  "${a.bits}|${a.ip.to_hex()}|${a.zone}|${a}|${bit(a.is_4())}${bit(a.is_6())}${bit(a.is_4in6())}${bit(a.is_loopback())}${bit(a.is_private())}${bit(a.is_unspecified())}${bit(a.is_link_local_unicast())}${bit(a.is_multicast())}|${a.unmap()}|${a.bytes().to_hex()}"
}

/// The addresses that parse, in order: the probes of PREFIXES.
fn good() -> List[netip.Addr] {
  var out: List[netip.Addr] = []
  for (s, _w) in ADDRS {
    if let Ok(a) = netip.parse_addr(s) {
      out.push(a)
    }
  }
  out
}

test "parse_addr gives Go's address, predicates, and errors" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in ADDRS {
    let got = match netip.parse_addr(s) {
      Ok(a) => addr_text(a)
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "parse_prefix gives Go's prefix, masked prefix, contains, and errors" {
  let g = good()
  var bad: List[(Str, Str, Str)] = []
  for (i, (s, want)) in PREFIXES.indexed() {
    let got = match netip.parse_prefix(s) {
      Ok(p) => {
        var cs = ""
        for k in 0..8 {
          cs = cs + bit(p.contains(g[(i * 8 + k) % g.len()]))
        }
        "${p}|${p.masked()}|${cs}"
      }
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "the order of Addr is Go's Compare" {
  let g = good()
  var bad: List[(Str, Str)] = []
  for i in 0..g.len() - 1 {
    let a = g[i]
    let b = g[i + 1]
    // Go: BitLen, then the 16 bytes, then the zone
    let want = if a.bits != b.bits { a.bits < b.bits } else if a.ip != b.ip { a.ip.to_hex() < b.ip.to_hex() } else { a.zone < b.zone }
    if (a < b) != want {
      bad.push(("${a}", "${b}"))
    }
  }
  expect(first_bad(bad)) == []
}

test "addr_from_bytes is Go's AddrFromSlice" {
  var bad: List[(Bytes, Str, Str)] = []
  for (b, want) in SLICES {
    let got = netip.addr_from_bytes(b).map(fn(a) { "${a}" }) ?? "none"
    if got != want {
      bad.push((b, got, want))
    }
  }
  expect(first_bad(bad)) == []
}
`
