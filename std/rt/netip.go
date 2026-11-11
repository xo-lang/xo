package rt

import (
	"encoding/binary"
	"net/netip"
)

// std/netip is merged and generated into std/rt (decisions 0588, 0597,
// std/rt_hosted.txt): its display methods are the port's. These are the
// runtime's own conversions to and from Go's net/netip, for client_ip and
// x509.

func netipOf(a netip.Addr) Netip_Addr {
	ip := a.As16()
	const sign = -1 << 63
	hi := int64(binary.BigEndian.Uint64(ip[:8]))
	lo := int64(binary.BigEndian.Uint64(ip[8:]))
	return Netip_Addr{F_bits: int64(a.BitLen()), F_hi: hi ^ sign, F_lo: lo ^ sign, F_zone: textOf(a.Zone())}
}

func (a Netip_Addr) goAddr() netip.Addr {
	var b [16]byte
	const sign = -1 << 63
	binary.BigEndian.PutUint64(b[:8], uint64(a.F_hi^sign))
	binary.BigEndian.PutUint64(b[8:], uint64(a.F_lo^sign))
	switch a.F_bits {
	case 32:
		return netip.AddrFrom16(b).Unmap()
	case 128:
		return netip.AddrFrom16(b).WithZone(a.F_zone)
	}
	return netip.Addr{}
}
