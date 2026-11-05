package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"

	amqp "github.com/Azure/go-amqp"
)

// frameRow is one frame the go-amqp client sent: the frame's bytes, as the client wrote them.
type frameRow struct {
	name string
	raw  []byte
}

// A tiny encoder for the frames the fake server sends back (only what go-amqp's client needs).
func wstr(s string) []byte { return append([]byte{0xa1, byte(len(s))}, s...) }
func wsym(s string) []byte { return append([]byte{0xa3, byte(len(s))}, s...) }
func wuint(n uint32) []byte {
	return append([]byte{0x70}, binary.BigEndian.AppendUint32(nil, n)...)
}
func wbool(b bool) []byte {
	if b {
		return []byte{0x41}
	}
	return []byte{0x42}
}
func wlist(fields ...[]byte) []byte {
	var body []byte
	for _, f := range fields {
		body = append(body, f...)
	}
	out := []byte{0xd0}
	out = binary.BigEndian.AppendUint32(out, uint32(len(body)+4))
	out = binary.BigEndian.AppendUint32(out, uint32(len(fields)))
	return append(out, body...)
}
func wdesc(code byte, fields ...[]byte) []byte {
	return append([]byte{0x00, 0x53, code}, wlist(fields...)...)
}
func wframe(typ byte, ch uint16, body []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(8+len(body)))
	out = append(out, 2, typ)
	out = binary.BigEndian.AppendUint16(out, ch)
	return append(out, body...)
}

var null = []byte{0x40}

// fakeServer reads frames the client sends, records them, and answers like a peer would.
type fakeServer struct {
	c    net.Conn
	rows []frameRow
	sasl bool
}

func (s *fakeServer) readFrame() ([]byte, error) {
	var h [4]byte
	if _, err := io.ReadFull(s.c, h[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(h[:])
	rest := make([]byte, n-4)
	if _, err := io.ReadFull(s.c, rest); err != nil {
		return nil, err
	}
	return append(h[:], rest...), nil
}

func perfCode(fr []byte) int {
	if len(fr) >= 11 && fr[8] == 0 && fr[9] == 0x53 {
		return int(fr[10])
	}
	return -1
}

var perfNames = map[int]string{0x10: "open", 0x11: "begin", 0x12: "attach", 0x13: "flow", 0x14: "transfer", 0x15: "disposition", 0x16: "detach", 0x17: "end", 0x18: "close", 0x41: "sasl-init"}

func (s *fakeServer) record(label string, fr []byte) {
	name := perfNames[perfCode(fr)]
	if name == "" {
		name = "empty"
	}
	s.rows = append(s.rows, frameRow{label + " " + name, fr})
}

func (s *fakeServer) run(label string, rcvMode bool) {
	hdr := make([]byte, 8)
	if s.sasl {
		io.ReadFull(s.c, hdr)
		s.c.Write([]byte("AMQP\x03\x01\x00\x00"))
		s.c.Write(wframe(1, 0, wdesc(0x40, wsym("PLAIN"))))
		fr, err := s.readFrame()
		if err != nil {
			return
		}
		s.record(label, fr)
		s.c.Write(wframe(1, 0, wdesc(0x44, []byte{0x50, 0})))
	}
	io.ReadFull(s.c, hdr)
	s.c.Write([]byte("AMQP\x00\x01\x00\x00"))
	handle := uint32(0)
	for {
		fr, err := s.readFrame()
		if err != nil {
			return
		}
		code := perfCode(fr)
		if code == 0x13 || code == 0x14 || code == 0x15 || code == 0x16 || code == 0x17 || code == 0x18 || code == 0x10 || code == 0x11 || code == 0x12 {
			s.record(label, fr)
		}
		switch code {
		case 0x10:
			s.c.Write(wframe(0, 0, wdesc(0x10, wstr("server"))))
		case 0x11:
			s.c.Write(wframe(0, 0, wdesc(0x11, []byte{0x60, 0, 0}, wuint(0), wuint(2048), wuint(2048))))
		case 0x12:
			// the client's attach: answer with the opposite role; a sender peer offers a message
			name := attachName(fr)
			clientIsSender := !bytes.Contains(fr[:min(len(fr), 40+len(name))], []byte{0x41}) && roleFalse(fr, name)
			if clientIsSender {
				s.c.Write(wframe(0, 0, wdesc(0x12, wstr(name), wuint(handle), wbool(true), null, null,
					wdesc(0x28, wstr("src")), wdesc(0x29, wstr("tgt")))))
				s.c.Write(wframe(0, 0, wdesc(0x13, wuint(0), wuint(2048), wuint(0), wuint(2048), wuint(handle), wuint(0), wuint(100))))
			} else {
				s.c.Write(wframe(0, 0, wdesc(0x12, wstr(name), wuint(handle), wbool(false), null, null,
					wdesc(0x28, wstr("src")), wdesc(0x29, wstr("tgt")), null, null, wuint(0))))
			}
		case 0x13:
			if rcvMode {
				msg := &amqp.Message{Data: [][]byte{[]byte("hi")}}
				b, _ := msg.MarshalBinary()
				body := wdesc(0x14, wuint(handle), wuint(0), []byte{0xa0, 1, 'a'}, wuint(0), wbool(false))
				s.c.Write(wframe(0, 0, append(body, b...)))
				rcvMode = false
			}
		case 0x14:
			s.c.Write(wframe(0, 0, wdesc(0x15, wbool(true), wuint(0), null, wbool(true), wdesc(0x24))))
		case 0x16:
			s.c.Write(wframe(0, 0, wdesc(0x16, wuint(handle), wbool(true))))
		case 0x17:
			s.c.Write(wframe(0, 0, wdesc(0x17)))
		case 0x18:
			s.c.Write(wframe(0, 0, wdesc(0x18)))
			return
		}
	}
}

// attachName reads the link name (the first field, a str8) of an attach frame.
func attachName(fr []byte) string {
	// frame: 8 header, 00 53 12, d0 size4 count4, a1 len name
	p := 8 + 3 + 1 + 4 + 4
	if fr[p] != 0xa1 {
		return ""
	}
	n := int(fr[p+1])
	return string(fr[p+2 : p+2+n])
}

// roleFalse reports whether the attach's role field (the third) is false (a sender).
func roleFalse(fr []byte, name string) bool {
	p := 8 + 3 + 1 + 4 + 4 + 2 + len(name)
	// handle: uint0 (0x43), smalluint (0x52 n) or uint (0x70 + 4)
	switch fr[p] {
	case 0x43:
		p++
	case 0x52:
		p += 2
	default:
		p += 5
	}
	return fr[p] == 0x42
}

func clientFrames() []frameRow {
	var all []frameRow
	run := func(label string, sasl bool, rcv bool, opts *amqp.ConnOptions, body func(c *amqp.Conn)) {
		cs, ss := net.Pipe()
		srv := &fakeServer{c: ss, sasl: sasl}
		done := make(chan struct{})
		go func() { srv.run(label, rcv); close(done) }()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		c, err := amqp.NewConn(ctx, cs, opts)
		if err != nil {
			panic(fmt.Sprintf("%s: NewConn: %v", label, err))
		}
		body(c)
		c.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		all = append(all, srv.rows...)
	}
	run("sasl", true, false, &amqp.ConnOptions{ContainerID: "oracle", SASLType: amqp.SASLTypePlain("user", "pass"), IdleTimeout: 60 * time.Second, Properties: map[string]any{"product": "oracle"}}, func(c *amqp.Conn) {})
	run("send", false, false, &amqp.ConnOptions{ContainerID: "oracle", HostName: "host"}, func(c *amqp.Conn) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sess, err := c.NewSession(ctx, nil)
		if err != nil {
			panic(err)
		}
		snd, err := sess.NewSender(ctx, "tgt", &amqp.SenderOptions{Name: "snd"})
		if err != nil {
			panic(err)
		}
		if err := snd.Send(ctx, amqp.NewMessage([]byte("hello")), nil); err != nil {
			panic(err)
		}
		snd.Close(ctx)
		sess.Close(ctx)
	})
	run("recv", false, true, &amqp.ConnOptions{ContainerID: "oracle"}, func(c *amqp.Conn) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sess, err := c.NewSession(ctx, nil)
		if err != nil {
			panic(err)
		}
		rcv, err := sess.NewReceiver(ctx, "src", &amqp.ReceiverOptions{Name: "rcv", Credit: 5})
		if err != nil {
			panic(err)
		}
		m, err := rcv.Receive(ctx, nil)
		if err != nil {
			panic(err)
		}
		if err := rcv.AcceptMessage(ctx, m); err != nil {
			panic(err)
		}
		rcv.Close(ctx)
		sess.Close(ctx)
	})
	return all
}
