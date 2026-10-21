package rt

import "net/netip"

// The display methods of std/netip's values (std.md 28): the runtime calls
// them through interfaces, so they stay Go methods (netipOf is the runtime's
// own conversion for client_ip); every other function is
// std/native/netip.xo on the Go backend too (decision 0312).

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

func (a Netip_Addr) M_display(*Task) string { return a.goAddr().String() }

func (p Netip_Prefix) M_display(*Task) string {
	return netip.PrefixFrom(p.F_addr.goAddr(), int(p.F_len)).String()
}
