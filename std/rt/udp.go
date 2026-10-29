package rt

import (
	"errors"
	"net"
	"strings"
	"sync"
)

// std/udp (decision 0406): UDP over Go's net, with Net.only's checks.

// udpConn is a UdpConn's socket and the Net it came from.
type udpConn struct {
	mu     sync.Mutex
	c      *net.UDPConn
	n      Net
	dialed bool
}

// Udp_UdpConn is `udp.UdpConn`.
type Udp_UdpConn struct{ u *udpConn }

// XoDebug implements Debug.
func (Udp_UdpConn) XoDebug(b *strings.Builder) { b.WriteString("<udp conn>") }

// udpAdmits is Net.only's check of addr (decision 0175), or nil.
func udpAdmits(n Net, addr string) *NetErr {
	if o, ok := n.(*onlyNet); ok && !o.admitsAddr(addr) {
		e := netDenied(addr)
		return &e
	}
	return nil
}

func udpErr(err error) NetErr {
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return netErr(3, "")
	}
	return netErr(4, err.Error())
}

func udpOpen(tk *Task, n Net, addr string, dial bool) Result[Udp_UdpConn, NetErr] {
	if tk.Canceled() {
		return Err[Udp_UdpConn](netErr(5, ""))
	}
	if e := udpAdmits(n, addr); e != nil {
		return Err[Udp_UdpConn](*e)
	}
	if fake, ok := n.(Testing_FakeNet); ok && fake.n.real == nil {
		op := "listen"
		if dial {
			op = "dial"
		}
		return Err[Udp_UdpConn](netErr(4, op+" udp "+addr+": the fake network has no UDP (xo test --real)"))
	}
	var c *net.UDPConn
	var err error
	if dial {
		var ra *net.UDPAddr
		ra, err = net.ResolveUDPAddr("udp", addr)
		if err == nil {
			c, err = net.DialUDP("udp", nil, ra)
		}
	} else {
		var pc net.PacketConn
		pc, err = net.ListenPacket("udp", addr)
		if err == nil {
			c = pc.(*net.UDPConn)
		}
	}
	if err != nil {
		return Err[Udp_UdpConn](netErr(4, err.Error()))
	}
	return Ok[Udp_UdpConn, NetErr](Udp_UdpConn{u: &udpConn{c: c, n: n, dialed: dial}})
}

// Udp_listen is `udp.listen(n, addr)`.
func Udp_listen(tk *Task, n Net, addr string) Result[Udp_UdpConn, NetErr] {
	return udpOpen(tk, n, addr, false)
}

// Udp_dial is `udp.dial(n, addr)`.
func Udp_dial(tk *Task, n Net, addr string) Result[Udp_UdpConn, NetErr] {
	return udpOpen(tk, n, addr, true)
}

// M_read_from is `c.read_from(max)`.
func (c Udp_UdpConn) M_read_from(tk *Task, max int64) Result[Tuple2[Bytes, string], NetErr] {
	if tk.Canceled() {
		return Err[Tuple2[Bytes, string]](netErr(5, ""))
	}
	if max < 1 {
		max = 1
	}
	if max > 65535 {
		max = 65535
	}
	buf := make([]byte, max)
	tk.leave()
	k, from, err := c.u.c.ReadFromUDP(buf)
	tk.enter()
	if err != nil && k == 0 {
		return Err[Tuple2[Bytes, string]](udpErr(err))
	}
	return Ok[Tuple2[Bytes, string], NetErr](Tuple2[Bytes, string]{F0: Bytes(buf[:k]), F1: from.String()})
}

// M_write_to is `c.write_to(data, addr)`.
func (c Udp_UdpConn) M_write_to(tk *Task, data Bytes, addr string) Result[Unit, NetErr] {
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	if e := udpAdmits(c.u.n, addr); e != nil {
		return Err[Unit](*e)
	}
	ra, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return Err[Unit](netErr(4, "write udp "+addr+": "+err.Error()))
	}
	if _, err := c.u.c.WriteToUDP([]byte(data), ra); err != nil {
		return Err[Unit](udpErr(err))
	}
	return Ok[Unit, NetErr](Unit{})
}

// M_read is `c.read(max)`.
func (c Udp_UdpConn) M_read(tk *Task, max int64) Result[Bytes, NetErr] {
	if !c.u.dialed {
		return Err[Bytes](netErr(4, "read udp: not connected"))
	}
	r := c.M_read_from(tk, max)
	if !r.Ok {
		return Err[Bytes](r.E)
	}
	return Ok[Bytes, NetErr](r.V.F0)
}

// M_write is `c.write(data)`.
func (c Udp_UdpConn) M_write(tk *Task, data Bytes) Result[Unit, NetErr] {
	if !c.u.dialed {
		return Err[Unit](netErr(4, "write udp: destination address required"))
	}
	if tk.Canceled() {
		return Err[Unit](netErr(5, ""))
	}
	if _, err := c.u.c.Write([]byte(data)); err != nil {
		return Err[Unit](udpErr(err))
	}
	return Ok[Unit, NetErr](Unit{})
}

// M_local_addr is `c.local_addr()`.
func (c Udp_UdpConn) M_local_addr(*Task) string { return boundAddr(c.u.c.LocalAddr()) }

// M_close is `c.close()`.
func (c Udp_UdpConn) M_close(*Task) {
	c.u.mu.Lock()
	defer c.u.mu.Unlock()
	c.u.c.Close()
}
