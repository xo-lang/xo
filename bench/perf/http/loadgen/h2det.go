package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

// h2Det makes tr speak HTTP/2 over a TLS connection it dials itself
// (prior knowledge, ALPN h2, so the URL's scheme becomes http) whose
// writes are held until a frame ends a stream: a request's frames and the
// control frames before it (a SETTINGS ack, a WINDOW_UPDATE) reach the
// server in one TLS write, whatever the goroutines' timing. net/http's
// client flushes a request's HEADERS and its DATA separately, so whether
// the server read them at once varied from run to run (callgrind.sh's h2
// scenario, about 5% between repeats on arm64).
func h2Det(tr *http.Transport, base string) string {
	tr.Protocols = new(http.Protocols)
	tr.Protocols.SetUnencryptedHTTP2(true)
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		raw, err := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}
		tc := tls.Client(raw, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}})
		if err := tc.HandshakeContext(ctx); err != nil {
			raw.Close()
			return nil, err
		}
		if tc.ConnectionState().NegotiatedProtocol != "h2" {
			tc.Close()
			return nil, errors.New("the server did not choose h2")
		}
		return &detConn{Conn: tc, scan: len(h2Preface)}, nil
	}
	return "http://" + base[len("https://"):]
}

const h2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

// detConn holds what the client writes until a HEADERS or DATA frame with
// END_STREAM completes a request, then writes all of it at once. A write
// held longer than holdMax goes out anyway (nothing in loadgen waits on a
// frame that does not end a stream, but a stuck client is worse than a
// noisy count).
type detConn struct {
	net.Conn
	mu    sync.Mutex
	buf   []byte
	scan  int // the offset of the first frame header not yet examined
	ended bool
	timer *time.Timer
	err   error
}

const holdMax = 200 * time.Millisecond

func (c *detConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return 0, c.err
	}
	c.buf = append(c.buf, p...)
	for c.scan+9 <= len(c.buf) {
		h := c.buf[c.scan:]
		n := int(h[0])<<16 | int(h[1])<<8 | int(h[2])
		if c.scan+9+n > len(c.buf) {
			break
		}
		// DATA (0) or HEADERS (1) with END_STREAM (0x1); GOAWAY (7) goes now
		if (h[3] <= 1 && h[4]&1 != 0) || h[3] == 7 {
			c.ended = true
		}
		c.scan += 9 + n
	}
	if c.ended && c.scan == len(c.buf) {
		c.flushLocked()
	} else if c.timer == nil {
		c.timer = time.AfterFunc(holdMax, func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.flushLocked()
		})
	}
	return len(p), c.err
}

// flushLocked writes the whole frames held (a frame still arriving stays).
func (c *detConn) flushLocked() {
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if c.scan > 0 && c.err == nil {
		_, c.err = c.Conn.Write(c.buf[:c.scan])
	}
	c.buf = append(c.buf[:0], c.buf[c.scan:]...)
	c.scan = 0
	c.ended = false
}

func (c *detConn) Close() error {
	c.mu.Lock()
	c.flushLocked()
	c.mu.Unlock()
	return c.Conn.Close()
}
