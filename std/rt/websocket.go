package rt

// std/websocket on the Go backend (decision 0313): gorilla/websocket's
// behavior (v1.5: conn.go advanceFrame, NextReader, WriteControl,
// handleProtocolError; server.go Upgrader.Upgrade; client.go Dialer),
// written here by hand (no third-party modules in std/rt), with the same
// steps as std/native/websocket.xo.

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Websocket_WsConn is a WebSocket connection, either side.
type Websocket_WsConn struct{ c *wsConn }

// XoDebug implements Debug.
func (Websocket_WsConn) XoDebug(b *strings.Builder) { b.WriteString("<websocket conn>") }

type wsConn struct {
	server bool
	conn   net.Conn
	max    int64
	proto  string

	rmu     sync.Mutex
	br      *bufio.Reader
	readErr *Websocket_WsErr

	wmu       sync.Mutex
	closeSent bool
	// writeErr is the error that stopped writing: it sticks, as
	// gorilla's (a frame may have gone out in part; decision 0430)
	writeErr *Websocket_WsErr

	// on_ping's and on_pong's functions (decision 0430), nil for none
	hmu          sync.Mutex
	pingH, pongH func(*Task, Bytes) Result[Unit, Never]
}

const (
	wsContinuation = 0
	wsText         = 1
	wsBinary       = 2
	wsClose        = 8
	wsPing         = 9
	wsPong         = 10
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func wsProtocolErr(msg string) Websocket_WsErr {
	return Websocket_WsErr{Tag: 2, P: Websocket_WsErr_Protocol{F_msg: msg}}
}

func wsClosed(code int64, reason string) Websocket_WsErr {
	return Websocket_WsErr{Tag: 1, P: Websocket_WsErr_Closed{F_code: code, F_reason: reason}}
}

func wsTooBig(limit int64) Websocket_WsErr {
	return Websocket_WsErr{Tag: 3, P: Websocket_WsErr_TooBig{F_limit: limit}}
}

func wsNetErr(err error) Websocket_WsErr {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		// past a deadline (decision 0430): Timeout
		return Websocket_WsErr{Tag: 4, P: Websocket_WsErr_Net{F_err: netErr(3, "")}}
	}
	return Websocket_WsErr{Tag: 4, P: Websocket_WsErr_Net{F_err: netErr(4, err.Error())}}
}

func wsHandshakeErr(status int64, msg string) Websocket_WsErr {
	return Websocket_WsErr{Tag: 5, P: Websocket_WsErr_Handshake{F_status: status, F_msg: msg}}
}

// wsCloseNames are gorilla's CloseError texts.
var wsCloseNames = map[int64]string{
	1000: " (normal)",
	1001: " (going away)",
	1002: " (protocol error)",
	1003: " (unsupported data)",
	1005: " (no status)",
	1006: " (abnormal closure)",
	1007: " (invalid payload data)",
	1008: " (policy violation)",
	1009: " (message too big)",
	1010: " (mandatory extension missing)",
	1011: " (internal server error)",
	1015: " (TLS handshake error)",
}

// M_display is gorilla's Error() of each error.
func (e Websocket_WsErr) M_display(tk *Task) string {
	switch p := e.P.(type) {
	case Websocket_WsErr_Closed:
		s := "websocket: close " + strconv.FormatInt(p.F_code, 10) + wsCloseNames[p.F_code]
		if p.F_reason != "" {
			s += ": " + p.F_reason
		}
		return s
	case Websocket_WsErr_Protocol:
		return "websocket: " + p.F_msg
	case Websocket_WsErr_TooBig:
		return "websocket: read limit exceeded"
	case Websocket_WsErr_Net:
		return Display(tk, p.F_err)
	case Websocket_WsErr_Handshake:
		return p.F_msg
	}
	return "websocket: error"
}

// wsValidCloseCode is gorilla's isValidReceivedCloseCode.
func wsValidCloseCode(code int64) bool {
	switch code {
	case 1000, 1001, 1002, 1003, 1007, 1008, 1009, 1010, 1011, 1012, 1013:
		return true
	}
	return code >= 3000 && code <= 4999
}

// wsCloseMessage is gorilla's FormatCloseMessage.
func wsCloseMessage(code int64, text string) []byte {
	if code == 1005 {
		return nil
	}
	b := make([]byte, 2+len(text))
	binary.BigEndian.PutUint16(b, uint16(code))
	copy(b[2:], text)
	return b
}

// writeFrame writes one final frame (masked from a client).
func (c *wsConn) writeFrame(op int, data []byte) *Websocket_WsErr {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.writeErr != nil {
		return c.writeErr
	}
	if c.closeSent {
		e := wsProtocolErr("close sent")
		return &e
	}
	n := len(data)
	hdr := make([]byte, 0, 14+n)
	hdr = append(hdr, 0x80|byte(op))
	mb := byte(0)
	if !c.server {
		mb = 0x80
	}
	switch {
	case n <= 125:
		hdr = append(hdr, mb|byte(n))
	case n <= 65535:
		hdr = append(hdr, mb|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, mb|127)
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	if c.server {
		hdr = append(hdr, data...)
	} else {
		var key [4]byte
		rand.Read(key[:])
		hdr = append(hdr, key[:]...)
		for i, b := range data {
			hdr = append(hdr, b^key[i&3])
		}
	}
	if op == wsClose {
		c.closeSent = true
	}
	if _, err := c.conn.Write(hdr); err != nil {
		e := wsNetErr(err)
		c.writeErr = &e
		return &e
	}
	return nil
}

// writeControl is gorilla's WriteControl: at most 125 bytes.
func (c *wsConn) writeControl(op int, data []byte) *Websocket_WsErr {
	if len(data) > 125 {
		e := wsProtocolErr("invalid control frame")
		return &e
	}
	return c.writeFrame(op, data)
}

// protocolError is gorilla's handleProtocolError: close code (1002) with
// msg, then the error.
func (c *wsConn) protocolError(code int64, msg string) *Websocket_WsErr {
	data := wsCloseMessage(code, msg)
	if len(data) > 125 {
		data = data[:125]
	}
	c.writeControl(wsClose, data)
	e := wsProtocolErr(msg)
	return &e
}

// readN reads n bytes; the end of the stream is gorilla's close 1006.
func (c *wsConn) readN(n int64) ([]byte, *Websocket_WsErr) {
	if n > 1<<20 {
		// read a long payload in pieces: a length is not an allocation
		var out []byte
		for n > 0 {
			k := min(n, 1<<20)
			p, err := c.readN(k)
			if err != nil {
				return nil, err
			}
			out = append(out, p...)
			n -= k
		}
		return out, nil
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(c.br, b); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			e := wsClosed(1006, io.ErrUnexpectedEOF.Error())
			return nil, &e
		}
		e := wsNetErr(err)
		return nil, &e
	}
	return b, nil
}

// readMessage reads frames up to a whole message (NextReader and its
// messageReader), handling control frames between them.
func (c *wsConn) readMessage(tk *Task) (Websocket_Message, *Websocket_WsErr) {
	kind := 0
	var data []byte
	checked := 0
	readFinal := true
	var length int64
	for {
		p, err := c.readN(2)
		if err != nil {
			return Websocket_Message{}, err
		}
		op := int(p[0] & 0xf)
		final := p[0]&0x80 != 0
		mask := p[1]&0x80 != 0
		n := int64(p[1] & 0x7f)
		var errs []string
		if p[0]&0x40 != 0 {
			errs = append(errs, "RSV1 set")
		}
		if p[0]&0x20 != 0 {
			errs = append(errs, "RSV2 set")
		}
		if p[0]&0x10 != 0 {
			errs = append(errs, "RSV3 set")
		}
		switch op {
		case wsClose, wsPing, wsPong:
			if n > 125 {
				errs = append(errs, "len > 125 for control")
			}
			if !final {
				errs = append(errs, "FIN not set on control")
			}
		case wsText, wsBinary:
			if !readFinal {
				errs = append(errs, "data before FIN")
			}
			readFinal = final
		case wsContinuation:
			if readFinal {
				errs = append(errs, "continuation after FIN")
			}
			readFinal = final
		default:
			errs = append(errs, "bad opcode "+strconv.Itoa(op))
		}
		if mask != c.server {
			errs = append(errs, "bad MASK")
		}
		if len(errs) > 0 {
			return Websocket_Message{}, c.protocolError(1002, strings.Join(errs, ", "))
		}
		switch n {
		case 126:
			q, err := c.readN(2)
			if err != nil {
				return Websocket_Message{}, err
			}
			n = int64(binary.BigEndian.Uint16(q))
		case 127:
			q, err := c.readN(8)
			if err != nil {
				return Websocket_Message{}, err
			}
			n = int64(binary.BigEndian.Uint64(q))
			if n < 0 {
				e := wsTooBig(c.max)
				return Websocket_Message{}, &e
			}
		}
		var key []byte
		if mask {
			if key, err = c.readN(4); err != nil {
				return Websocket_Message{}, err
			}
		}
		if op == wsText || op == wsBinary || op == wsContinuation {
			length += n
			if length < 0 || (c.max > 0 && length > c.max) {
				c.writeControl(wsClose, wsCloseMessage(1009, ""))
				e := wsTooBig(c.max)
				return Websocket_Message{}, &e
			}
		}
		var payload []byte
		if op == wsText || (op == wsContinuation && kind == wsText) {
			// a text payload is validated in the pieces that arrive
			// (Autobahn 6.4.3, 6.4.4): a bad sequence fails the
			// connection before the rest of the frame is read
			for got := int64(0); got < n; {
				if c.br.Buffered() == 0 {
					if _, perr := c.br.Peek(1); perr != nil {
						if perr == io.EOF || perr == io.ErrUnexpectedEOF {
							e := wsClosed(1006, io.ErrUnexpectedEOF.Error())
							return Websocket_Message{}, &e
						}
						e := wsNetErr(perr)
						return Websocket_Message{}, &e
					}
				}
				piece, err := c.readN(min(int64(c.br.Buffered()), n-got))
				if err != nil {
					return Websocket_Message{}, err
				}
				if mask {
					for i := range piece {
						piece[i] ^= key[(got+int64(i))&3]
					}
				}
				data = append(data, piece...)
				cut, ok := wsUTF8Prefix(data[checked:])
				if !ok {
					return Websocket_Message{}, c.protocolError(1007, "invalid UTF-8 in text frame")
				}
				checked += cut
				got += int64(len(piece))
			}
		} else {
			var err *Websocket_WsErr
			if payload, err = c.readN(n); err != nil {
				return Websocket_Message{}, err
			}
			if mask {
				for i := range payload {
					payload[i] ^= key[i&3]
				}
			}
		}
		switch op {
		case wsPong:
			c.hmu.Lock()
			h := c.pongH
			c.hmu.Unlock()
			if h != nil {
				h(tk, Bytes(payload))
			}
			continue
		case wsPing:
			c.hmu.Lock()
			h := c.pingH
			c.hmu.Unlock()
			if h != nil {
				h(tk, Bytes(payload))
				continue
			}
			if e := c.writeControl(wsPong, payload); e != nil && !c.isCloseSent(e) {
				return Websocket_Message{}, e
			}
			continue
		case wsClose:
			code := int64(1005)
			text := ""
			if len(payload) >= 2 {
				code = int64(binary.BigEndian.Uint16(payload))
				if !wsValidCloseCode(code) {
					return Websocket_Message{}, c.protocolError(1002, "bad close code "+strconv.FormatInt(code, 10))
				}
				text = string(payload[2:])
				if !utf8.ValidString(text) {
					return Websocket_Message{}, c.protocolError(1002, "invalid utf8 payload in close frame")
				}
			}
			c.writeControl(wsClose, wsCloseMessage(code, ""))
			e := wsClosed(code, text)
			return Websocket_Message{}, &e
		case wsText, wsBinary:
			kind = op
		}
		data = append(data, payload...)
		if kind == wsText {
			// each fragment is checked as it arrives (gorilla's utf8
			// reader): a bad sequence fails the connection at its
			// fragment, an incomplete one at the end waits for the next
			cut, ok := wsUTF8Prefix(data[checked:])
			if !ok {
				return Websocket_Message{}, c.protocolError(1007, "invalid UTF-8 in text frame")
			}
			checked += cut
		}
		if readFinal {
			if checked != len(data) && kind == wsText {
				return Websocket_Message{}, c.protocolError(1007, "invalid UTF-8 in text frame")
			}
			return Websocket_Message{F_kind: Websocket_Kind{Tag: kind}, F_data: Bytes(data)}, nil
		}
	}
}

// wsUTF8Prefix is how many bytes of b are whole characters; ok is false
// when b has an invalid sequence (an incomplete one at the end is not).
func wsUTF8Prefix(b []byte) (cut int, ok bool) {
	for cut < len(b) {
		if b[cut] < utf8.RuneSelf {
			cut++
			continue
		}
		r, size := utf8.DecodeRune(b[cut:])
		if r == utf8.RuneError && size == 1 {
			return cut, !utf8.FullRune(b[cut:])
		}
		cut += size
	}
	return cut, true
}

func (c *wsConn) isCloseSent(e *Websocket_WsErr) bool {
	p, ok := e.P.(Websocket_WsErr_Protocol)
	return ok && p.F_msg == "close sent"
}

// M_read is `c.read()`: the next whole message; an error sticks.
func (w Websocket_WsConn) M_read(tk *Task) Result[Websocket_Message, Websocket_WsErr] {
	c := w.c
	if tk.Canceled() {
		return Err[Websocket_Message](wsNetErr(errors.New("canceled")))
	}
	c.rmu.Lock()
	defer c.rmu.Unlock()
	if c.readErr != nil {
		return Err[Websocket_Message](*c.readErr)
	}
	m, err := c.readMessage(tk)
	if err != nil {
		c.readErr = err
		return Err[Websocket_Message](*err)
	}
	return Ok[Websocket_Message, Websocket_WsErr](m)
}

// M_write is `c.write(m)`: one frame.
func (w Websocket_WsConn) M_write(tk *Task, m Websocket_Message) Result[Unit, Websocket_WsErr] {
	op := wsBinary
	if m.F_kind.Tag == 1 {
		op = wsText
	}
	if e := w.c.writeFrame(op, []byte(m.F_data)); e != nil {
		return Err[Unit](*e)
	}
	return Ok[Unit, Websocket_WsErr](Unit{})
}

// M_write_text is `c.write_text(s)`.
func (w Websocket_WsConn) M_write_text(tk *Task, s string) Result[Unit, Websocket_WsErr] {
	if e := w.c.writeFrame(wsText, []byte(s)); e != nil {
		return Err[Unit](*e)
	}
	return Ok[Unit, Websocket_WsErr](Unit{})
}

// M_ping is `c.ping(data)`.
func (w Websocket_WsConn) M_ping(tk *Task, data Bytes) Result[Unit, Websocket_WsErr] {
	if e := w.c.writeControl(wsPing, []byte(data)); e != nil {
		return Err[Unit](*e)
	}
	return Ok[Unit, Websocket_WsErr](Unit{})
}

// M_close is `c.close(code, reason)`: the close frame (unless one was
// sent), then the connection.
func (w Websocket_WsConn) M_close(tk *Task, code int64, reason string) Result[Unit, Websocket_WsErr] {
	c := w.c
	var e *Websocket_WsErr
	if len(reason) > 123 {
		x := wsProtocolErr("invalid control frame")
		e = &x
	} else {
		c.wmu.Lock()
		sent := c.closeSent
		c.wmu.Unlock()
		if !sent {
			e = c.writeControl(wsClose, wsCloseMessage(code, reason))
		}
	}
	c.conn.Close()
	if e != nil {
		return Err[Unit](*e)
	}
	return Ok[Unit, Websocket_WsErr](Unit{})
}

// M_subprotocol is `c.subprotocol()`.
func (w Websocket_WsConn) M_subprotocol(*Task) string { return w.c.proto }

// M_on_ping is `c.on_ping(f)` (decision 0430): gorilla's SetPingHandler.
func (w Websocket_WsConn) M_on_ping(tk *Task, f func(*Task, Bytes) Result[Unit, Never]) {
	w.c.hmu.Lock()
	w.c.pingH = f
	w.c.hmu.Unlock()
}

// M_on_pong is `c.on_pong(f)` (decision 0430): gorilla's SetPongHandler.
func (w Websocket_WsConn) M_on_pong(tk *Task, f func(*Task, Bytes) Result[Unit, Never]) {
	w.c.hmu.Lock()
	w.c.pongH = f
	w.c.hmu.Unlock()
}

// M_pong is `c.pong(data)`: a pong control frame.
func (w Websocket_WsConn) M_pong(tk *Task, data Bytes) Result[Unit, Websocket_WsErr] {
	if e := w.c.writeControl(wsPong, []byte(data)); e != nil {
		return Err[Unit](*e)
	}
	return Ok[Unit, Websocket_WsErr](Unit{})
}

// M_set_deadline is `c.set_deadline(clock, t)` (decision 0430):
// gorilla's SetReadDeadline and SetWriteDeadline, the connection's.
func (w Websocket_WsConn) M_set_deadline(tk *Task, clock Clock, t Option[Time]) {
	Conn{c: w.c.conn}.setDeadline(tk, clock, t, true, true)
}

// M_set_read_deadline is gorilla's SetReadDeadline (decision 0430).
func (w Websocket_WsConn) M_set_read_deadline(tk *Task, clock Clock, t Option[Time]) {
	Conn{c: w.c.conn}.setDeadline(tk, clock, t, true, false)
}

// M_set_write_deadline is gorilla's SetWriteDeadline (decision 0430).
func (w Websocket_WsConn) M_set_write_deadline(tk *Task, clock Clock, t Option[Time]) {
	Conn{c: w.c.conn}.setDeadline(tk, clock, t, false, true)
}

// wsTokens reports whether a field of name in vs lists token (gorilla's
// tokenListContainsValue: comma separated, case insensitive).
func wsTokens(vs []string, token string) bool {
	for _, v := range vs {
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

func wsAcceptKey(key string) string {
	h := sha1.Sum([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h[:])
}

// wsValidKey is gorilla's isValidChallengeKey.
func wsValidKey(s string) bool {
	if s == "" {
		return false
	}
	b, err := base64.StdEncoding.DecodeString(s)
	return err == nil && len(b) == 16
}

// wsRefuse is gorilla's returnError: the status text.
func wsRefuse(tk *Task, status int64) Http_Response {
	h := mapCap[string, string](3)
	Map_put(tk, &h, "Sec-Websocket-Version", "13")
	Map_put(tk, &h, "Content-Type", "text/plain; charset=utf-8")
	Map_put(tk, &h, "X-Content-Type-Options", "nosniff")
	return Http_Response{F_status: status, F_headers: h, F_body: Bytes(http.StatusText(int(status)) + "\n")}
}

// Websocket_upgrade is `websocket.upgrade(req, f, ...)`: gorilla's
// Upgrader checks, then a 101 response whose stream is the hand-off
// (http_net.go hijacks the connection for it).
func Websocket_upgrade(tk *Task, req Http_Request, f func(*Task, Websocket_WsConn) Result[Unit, Never], protocols List[string], origins List[string], maxMessage int64) Http_Response {
	hv := func(name string) []string { return req.M_header_values(tk, name).s }
	if !wsTokens(hv("Connection"), "upgrade") || !wsTokens(hv("Upgrade"), "websocket") {
		return wsRefuse(tk, 400)
	}
	if req.F_method != "GET" {
		return wsRefuse(tk, 405)
	}
	if !wsTokens(hv("Sec-Websocket-Version"), "13") {
		return wsRefuse(tk, 400)
	}
	if !wsOriginOK(req, hv("Origin"), origins.s) {
		return wsRefuse(tk, 403)
	}
	key := ""
	if ks := hv("Sec-Websocket-Key"); len(ks) > 0 {
		key = ks[0]
	}
	if !wsValidKey(key) {
		return wsRefuse(tk, 400)
	}
	proto := ""
	offered := wsProtocols(hv("Sec-Websocket-Protocol"))
	for _, sp := range protocols.s {
		for _, cp := range offered {
			if cp == sp && proto == "" {
				proto = cp
			}
		}
	}
	return Http_Response{F_status: 101, XStream: func(tk *Task, bw Http_BodyWriter) Result[Unit, IoErr] {
		if bw.w == nil || bw.w.rw == nil {
			// Router.call: no connection to hand over
			return Ok[Unit, IoErr](Unit{})
		}
		hj, ok := bw.w.rw.(http.Hijacker)
		if !ok {
			return Ok[Unit, IoErr](Unit{})
		}
		conn, brw, err := hj.Hijack()
		if err != nil {
			return Ok[Unit, IoErr](Unit{})
		}
		defer conn.Close()
		conn.SetDeadline(time.Time{})
		var b strings.Builder
		b.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ")
		b.WriteString(wsAcceptKey(key))
		b.WriteString("\r\n")
		if proto != "" {
			b.WriteString("Sec-WebSocket-Protocol: " + proto + "\r\n")
		}
		b.WriteString("\r\n")
		if _, err := conn.Write([]byte(b.String())); err != nil {
			return Ok[Unit, IoErr](Unit{})
		}
		c := &wsConn{server: true, conn: conn, br: brw.Reader, max: maxMessage, proto: proto}
		f(tk, Websocket_WsConn{c: c})
		return Ok[Unit, IoErr](Unit{})
	}}
}

// wsProtocols is gorilla's Subprotocols: the offered tokens.
func wsProtocols(vs []string) []string {
	var out []string
	for _, v := range vs {
		for _, t := range strings.Split(v, ",") {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// wsOriginOK is gorilla's checkSameOrigin (the host names compared: a
// Request keeps no port of its Host field), or an allowed host.
func wsOriginOK(req Http_Request, origin []string, allowed []string) bool {
	if len(origin) == 0 {
		return true
	}
	u, err := url.Parse(origin[0])
	if err != nil {
		return false
	}
	for _, a := range allowed {
		if a == "*" || strings.EqualFold(a, u.Host) || strings.EqualFold(a, u.Hostname()) {
			return true
		}
	}
	return strings.EqualFold(u.Hostname(), req.F_url.F_host)
}

// Websocket_dial is `websocket.dial(net, url, ...)`: gorilla's Dialer.
func Websocket_dial(tk *Task, n Net, rawURL string, headers Map[string, string], protocols List[string], maxMessage int64, roots Bytes) Result[Websocket_WsConn, Websocket_WsErr] {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return Err[Websocket_WsConn](wsHandshakeErr(0, "websocket: malformed ws or wss URL"))
	}
	addr := u.Host
	if u.Port() == "" {
		port := "80"
		if u.Scheme == "wss" {
			port = "443"
		}
		addr = net.JoinHostPort(u.Hostname(), port)
	}
	d := n.M_dial(tk, addr)
	if !d.Ok {
		return Err[Websocket_WsConn](Websocket_WsErr{Tag: 4, P: Websocket_WsErr_Net{F_err: d.E}})
	}
	conn := d.V.c
	if u.Scheme == "wss" {
		// std/tls's client (no ALPN: the server answers HTTP/1.1)
		var r Option[Tls_Roots]
		if len(roots) > 0 {
			rr := Tls_roots(tk, roots)
			if !rr.Ok {
				conn.Close()
				return Err[Websocket_WsConn](wsNetErr(errors.New(rr.E.F_msg)))
			}
			r = Some(rr.V)
		}
		tc := Tls_client(tk, d.V, u.Hostname(), r, Tls_Verify{Tag: 1}, List[string]{})
		if !tc.Ok {
			return Err[Websocket_WsConn](Websocket_WsErr{Tag: 4, P: Websocket_WsErr_Net{F_err: tc.E}})
		}
		if tc.V.t != nil {
			conn = tc.V.t
		} else {
			// a FakeNet connection: no TLS on the wire
			conn = tc.V.c
		}
	}
	var kb [16]byte
	rand.Read(kb[:])
	key := base64.StdEncoding.EncodeToString(kb[:])
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	var b strings.Builder
	b.WriteString("GET " + path + " HTTP/1.1\r\nHost: " + u.Host + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n")
	if len(protocols.s) > 0 {
		b.WriteString("Sec-WebSocket-Protocol: " + strings.Join(protocols.s, ", ") + "\r\n")
	}
	for k, v := range Map_All(headers) {
		b.WriteString(k + ": " + v + "\r\n")
	}
	b.WriteString("\r\n")
	if _, err := conn.Write([]byte(b.String())); err != nil {
		conn.Close()
		return Err[Websocket_WsConn](wsNetErr(err))
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return Err[Websocket_WsConn](wsHandshakeErr(0, "websocket: bad handshake"))
	}
	if resp.StatusCode != 101 || !wsTokens(resp.Header.Values("Upgrade"), "websocket") || !wsTokens(resp.Header.Values("Connection"), "upgrade") || resp.Header.Get("Sec-Websocket-Accept") != wsAcceptKey(key) {
		conn.Close()
		return Err[Websocket_WsConn](wsHandshakeErr(int64(resp.StatusCode), "websocket: bad handshake"))
	}
	c := &wsConn{conn: conn, br: br, max: maxMessage, proto: resp.Header.Get("Sec-Websocket-Protocol")}
	return Ok[Websocket_WsConn, Websocket_WsErr](Websocket_WsConn{c: c})
}
