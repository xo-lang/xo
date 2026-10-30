package rt

import (
	"net"
	"sort"
)

// M_interfaces is `n.interfaces()` (decision 0426): Go's net.Interfaces
// with each one's Addrs, sorted by index.
func (realNet) M_interfaces(tk *Task) Result[List[NetInterface], NetErr] {
	return fx(tk, "net.interfaces", false, func() []any { return nil }, func() Result[List[NetInterface], NetErr] {
		ifs, err := net.Interfaces()
		if err != nil {
			return Err[List[NetInterface]](netErr(4, err.Error()))
		}
		sort.Slice(ifs, func(i, j int) bool { return ifs[i].Index < ifs[j].Index })
		out := make([]NetInterface, 0, len(ifs))
		for _, ifi := range ifs {
			var addrs []string
			if as, err := ifi.Addrs(); err == nil {
				for _, a := range as {
					addrs = append(addrs, a.String())
				}
			}
			f := ifi.Flags
			out = append(out, NetInterface{
				F_index:         int64(ifi.Index),
				F_name:          ifi.Name,
				F_mtu:           int64(ifi.MTU),
				F_hardware_addr: ifi.HardwareAddr.String(),
				F_flags: InterfaceFlags{
					F_up:             f&net.FlagUp != 0,
					F_broadcast:      f&net.FlagBroadcast != 0,
					F_loopback:       f&net.FlagLoopback != 0,
					F_point_to_point: f&net.FlagPointToPoint != 0,
					F_multicast:      f&net.FlagMulticast != 0,
					F_running:        f&net.FlagRunning != 0,
				},
				F_addrs: ListFromSlice(addrs),
			})
		}
		return Ok[List[NetInterface], NetErr](ListFromSlice(out))
	})
}

// An attenuated Net answers interfaces too: it names no remote host.
func (n *onlyNet) M_interfaces(tk *Task) Result[List[NetInterface], NetErr] {
	return n.inner.M_interfaces(tk)
}

// M_interfaces of a FakeNet is one loopback interface, as the native
// FakeNet's.
func (f Testing_FakeNet) M_interfaces(tk *Task) Result[List[NetInterface], NetErr] {
	if f.n.real != nil {
		return f.n.real.M_interfaces(tk)
	}
	lo := NetInterface{F_index: 1, F_name: "lo", F_mtu: 65536, F_flags: InterfaceFlags{F_up: true, F_loopback: true, F_multicast: true, F_running: true}, F_addrs: ListOf("127.0.0.1/8", "::1/128")}
	return Ok[List[NetInterface], NetErr](ListOf(lo))
}
