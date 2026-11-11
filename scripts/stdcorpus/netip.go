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
	o.WriteString("]\n\n/// (text, Go's result: STRING|MASKED|contains of each probe, then Go's STRING again as display, or err TEXT)\nconst PREFIXES: List[(Str, Str)] = [\n")
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
			w = textOf(p.String()) + "|" + textOf(p.Masked().String()) + "|" + string(cs) + "|" + textOf(p.String())
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
	netipMore(&o, r, good)
	o.WriteString(netipTests)
	return o.String()
}

// netipMore writes AddrPort, Addr.Prefix, Next, Prev, WithZone,
// PrefixFrom, Prefix.Bits, IsSingleIP and Overlaps on the parsed
// addresses and random texts.
func netipMore(o *strings.Builder, r *rand.Rand, good []netip.Addr) {
	o.WriteString("\n/// (text, Go's ParseAddrPort: ADDR|PORT|STRING, or err TEXT)\nconst ADDR_PORTS: List[(Str, Str)] = [\n")
	ports := []string{"0", "80", "443", "65535", "65536", "080", "", "-1", "+1", "x", "99999", "1_0"}
	for i := 0; i < 1500; i++ {
		a := good[r.Intn(len(good))]
		h := a.String()
		switch r.Intn(6) {
		case 0, 1, 2:
			if a.Is6() {
				h = "[" + h + "]"
			}
		case 3:
			h = "[" + h + "]"
		case 4:
		case 5:
			h = []string{"", "[", "[]", "[::1", "1.2.3.4]", "host"}[r.Intn(6)]
		}
		port := ports[r.Intn(len(ports))]
		if r.Intn(2) == 0 {
			port = strconv.Itoa(r.Intn(65536))
		}
		s := h + ":" + port
		if r.Intn(30) == 0 {
			s = h
		}
		s = textOf(s)
		w := ""
		if ap, err := netip.ParseAddrPort(s); err != nil {
			w = "err " + textOf(err.Error())
		} else {
			w = textOf(ap.Addr().String()) + "|" + strconv.Itoa(int(ap.Port())) + "|" + textOf(ap.String())
		}
		fmt.Fprintf(o, "  (%s, %s),\n", xq(s), xq(w))
	}
	o.WriteString("]\n\n/// (index into good(), bits, Go's Addr.Prefix: STRING or err TEXT; Next; Prev (STRING or none); PrefixFrom (STRING|BITS|IsSingleIP or none); AddrPortFrom(a, port).String())\nconst ADDR_OPS: List[(Int, Int, Int, Str)] = [\n")
	for i := 0; i < 1500; i++ {
		k := r.Intn(len(good))
		a := good[k]
		if r.Intn(8) == 0 {
			// the ends of each family, where Next and Prev run out
			ends := []string{"0.0.0.0", "255.255.255.255", "::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "::ffff:255.255.255.255", "::ffff:0.0.0.0", "0:0:0:0:ffff:ffff:ffff:ffff", "::1:0:0:0:0", "fe80::ffff:ffff:ffff:ffff%eth0"}
			e := ends[r.Intn(len(ends))]
			for j, g := range good {
				if g.String() == e {
					k, a = j, g
				}
			}
		}
		bits := []int{r.Intn(33), r.Intn(129), -1, 0, 32, 33, 128, 129}[r.Intn(8)]
		var parts []string
		if p, err := a.Prefix(bits); err != nil {
			parts = append(parts, "err "+err.Error())
		} else {
			parts = append(parts, p.String())
		}
		for _, x := range []netip.Addr{a.Next(), a.Prev()} {
			if x.IsValid() {
				parts = append(parts, x.String())
			} else {
				parts = append(parts, "none")
			}
		}
		if p := netip.PrefixFrom(a, bits); p.IsValid() {
			parts = append(parts, fmt.Sprintf("%s|%d|%v", p, p.Bits(), p.IsSingleIP()))
		} else {
			parts = append(parts, "none")
		}
		port := r.Intn(65536)
		parts = append(parts, netip.AddrPortFrom(a, uint16(port)).String())
		fmt.Fprintf(o, "  (%d, %d, %d, %s),\n", k, bits, port, xq(textOf(strings.Join(parts, ";"))))
	}
	o.WriteString("]\n\n/// (index into good(), zone, Go's WithZone(zone).String())\nconst ZONES: List[(Int, Str, Str)] = [\n")
	for i := 0; i < 200; i++ {
		k := r.Intn(len(good))
		z := []string{"", "eth0", "1", "en0"}[r.Intn(4)]
		fmt.Fprintf(o, "  (%d, %s, %s),\n", k, xq(z), xq(textOf(good[k].WithZone(z).String())))
	}
	o.WriteString("]\n\n/// (prefix, prefix, Go's Overlaps)\nconst OVERLAPS: List[(Str, Str, Bool)] = [\n")
	pfx := func() netip.Prefix {
		for {
			a := good[r.Intn(len(good))]
			if r.Intn(3) == 0 && len(good) > 0 {
				// a near neighbour: shares a long run of leading bits
				a = good[r.Intn(len(good))]
			}
			n := r.Intn(a.BitLen() + 1)
			if r.Intn(3) == 0 {
				n = r.Intn(9)
			}
			p, err := a.WithZone("").Prefix(n)
			if err == nil {
				if r.Intn(2) == 0 {
					return netip.PrefixFrom(a.WithZone(""), n)
				}
				return p
			}
		}
	}
	for i := 0; i < 1500; i++ {
		p := pfx()
		q := pfx()
		if r.Intn(3) == 0 {
			// q within or around p
			b := p.Bits() + r.Intn(9) - 4
			if b < 0 {
				b = 0
			}
			if qq, err := p.Addr().Prefix(b); err == nil {
				q = qq
			}
		}
		fmt.Fprintf(o, "  (%s, %s, %v),\n", xq(p.String()), xq(q.String()), p.Overlaps(q))
	}
	o.WriteString("]\n")
}

const netipTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn bit(x: Bool) -> Str { if x { "1" } else { "0" } }

/// Go's As16 as hex: the 16 bytes of the address (an IPv4 address as ::ffff:a.b.c.d).
fn ip16(a: netip.Addr) -> Str {
  if a.bits == 32 {
    return "00000000000000000000ffff" + a.bytes().to_hex()
  }
  if a.bits == 128 {
    return a.bytes().to_hex()
  }
  "00000000000000000000000000000000"
}

fn addr_text(a: netip.Addr) -> Str {
  "${a.bits}|${ip16(a)}|${a.zone}|${a}|${bit(a.is_4())}${bit(a.is_6())}${bit(a.is_4in6())}${bit(a.is_loopback())}${bit(a.is_private())}${bit(a.is_unspecified())}${bit(a.is_link_local_unicast())}${bit(a.is_multicast())}|${a.unmap()}|${a.bytes().to_hex()}"
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

test "parse_addr_port gives Go's AddrPort and errors" {
  var bad: List[(Str, Str, Str)] = []
  for (s, want) in ADDR_PORTS {
    let got = match netip.parse_addr_port(s) {
      Ok(p) => "${p.addr}|${p.port}|${p}"
      Err(e) => "err ${e.display()}"
    }
    if got != want {
      bad.push((s, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "Addr.prefix, next, prev, prefix_from and addr_port are Go's" {
  let g = good()
  var bad: List[(Str, Int, Str, Str)] = []
  for (k, bits, port, want) in ADDR_OPS {
    let a = g[k]
    let pre = match a.prefix(bits) {
      Ok(p) => "${p}"
      Err(e) => "err ${e.display()}"
    }
    let nx = a.next().map(fn(x) { "${x}" }) ?? "none"
    let pv = a.prev().map(fn(x) { "${x}" }) ?? "none"
    let pf = netip.prefix_from(a, bits).map(fn(p) { "${p}|${p.bits()}|${p.is_single_ip()}" }) ?? "none"
    let got = "${pre};${nx};${pv};${pf};${netip.addr_port(a, port)}"
    if got != want {
      bad.push(("${a}", bits, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "with_zone is Go's WithZone" {
  let g = good()
  var bad: List[(Str, Str, Str)] = []
  for (k, z, want) in ZONES {
    let got = "${g[k].with_zone(z)}"
    if got != want {
      bad.push(("${g[k]}", z, got))
    }
  }
  expect(first_bad(bad)) == []
}

test "Prefix.overlaps is Go's Overlaps" {
  var bad: List[(Str, Str, Bool)] = []
  for (a, b, want) in OVERLAPS {
    let p = netip.parse_prefix(a)?
    let q = netip.parse_prefix(b)?
    if p.overlaps(q) != want || q.overlaps(p) != want {
      bad.push((a, b, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "AddrPort orders by address, then port" {
  let a = netip.parse_addr_port("1.2.3.4:80")?
  let b = netip.parse_addr_port("1.2.3.4:443")?
  let c = netip.parse_addr_port("[::1]:1")?
  expect(a < b) == true
  expect(b < c) == true
  expect(netip.addr_port(a.addr, 65536 + 80)) == a
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
        "${p}|${p.masked()}|${cs}|${p.display()}"
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
    let want = if a.bits != b.bits { a.bits < b.bits } else if ip16(a) != ip16(b) { ip16(a) < ip16(b) } else { a.zone < b.zone }
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
