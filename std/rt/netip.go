package rt

import "net/netip"

// std/netip (std.md 28, decision 0287): Go's net/netip, its Addr as Xo's
// value (BitLen, As16, Zone) and back.

func (e Netip_ParseErr) M_display(*Task) string { return e.F_msg }

func netipOf(a netip.Addr) Netip_Addr {
	ip := a.As16()
	return Netip_Addr{F_bits: int64(a.BitLen()), F_ip: Bytes(ip[:]), F_zone: textOf(a.Zone())}
}

func (a Netip_Addr) goAddr() netip.Addr {
	var b [16]byte
	copy(b[:], a.F_ip)
	switch a.F_bits {
	case 32:
		return netip.AddrFrom16(b).Unmap()
	case 128:
		return netip.AddrFrom16(b).WithZone(a.F_zone)
	}
	return netip.Addr{}
}

func Netip_parse_addr(_ *Task, s string) Result[Netip_Addr, Netip_ParseErr] {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return Err[Netip_Addr](Netip_ParseErr{F_msg: textOf(err.Error())})
	}
	return Ok[Netip_Addr, Netip_ParseErr](netipOf(a))
}

func Netip_parse_prefix(_ *Task, s string) Result[Netip_Prefix, Netip_ParseErr] {
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return Err[Netip_Prefix](Netip_ParseErr{F_msg: textOf(err.Error())})
	}
	return Ok[Netip_Prefix, Netip_ParseErr](Netip_Prefix{F_addr: netipOf(p.Addr()), F_len: int64(p.Bits())})
}

func Netip_addr_from_bytes(_ *Task, b Bytes) Option[Netip_Addr] {
	a, ok := netip.AddrFromSlice([]byte(b))
	if !ok {
		return None[Netip_Addr]()
	}
	return Some(netipOf(a))
}

func (a Netip_Addr) M_is_4(*Task) bool           { return a.goAddr().Is4() }
func (a Netip_Addr) M_is_6(*Task) bool           { return a.goAddr().Is6() }
func (a Netip_Addr) M_is_4in6(*Task) bool        { return a.goAddr().Is4In6() }
func (a Netip_Addr) M_unmap(*Task) Netip_Addr    { return netipOf(a.goAddr().Unmap()) }
func (a Netip_Addr) M_is_loopback(*Task) bool    { return a.goAddr().IsLoopback() }
func (a Netip_Addr) M_is_private(*Task) bool     { return a.goAddr().IsPrivate() }
func (a Netip_Addr) M_is_unspecified(*Task) bool { return a.goAddr().IsUnspecified() }
func (a Netip_Addr) M_is_multicast(*Task) bool   { return a.goAddr().IsMulticast() }
func (a Netip_Addr) M_bytes(*Task) Bytes         { return Bytes(a.goAddr().AsSlice()) }
func (a Netip_Addr) M_display(*Task) string      { return a.goAddr().String() }
func (a Netip_Addr) M_is_link_local_unicast(*Task) bool {
	return a.goAddr().IsLinkLocalUnicast()
}

func (p Netip_Prefix) goPrefix() netip.Prefix {
	return netip.PrefixFrom(p.F_addr.goAddr(), int(p.F_len))
}

func (p Netip_Prefix) M_contains(_ *Task, a Netip_Addr) bool {
	return p.goPrefix().Contains(a.goAddr())
}

func (p Netip_Prefix) M_masked(*Task) Netip_Prefix {
	m := p.goPrefix().Masked()
	if !m.IsValid() {
		return Netip_Prefix{F_addr: netipOf(netip.Addr{}), F_len: p.F_len}
	}
	return Netip_Prefix{F_addr: netipOf(m.Addr()), F_len: int64(m.Bits())}
}

func (p Netip_Prefix) M_display(*Task) string { return p.goPrefix().String() }
