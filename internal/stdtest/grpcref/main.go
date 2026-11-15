// grpcref is the oracle for std/grpc's pure layer (decision 0535): it reads lines "op<TAB>arg" and prints what grpc-go
// makes of them, observed on the wire since grpc-go's helpers are internal:
//
//	timeout NS   the grpc-timeout header a grpc-go client sends for a context deadline of NS nanoseconds
//	message TEXT the grpc-message trailer (percent encoded) a grpc-go server sends for status.Error(code, TEXT)
//	decode ESC   the status message (hex of its bytes) a grpc-go client reports for a response whose grpc-message trailer is ESC
package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// rawServer answers every stream with the headers and trailers h, and reports the request headers on got.
func rawServer(trailerMsg string, got chan<- map[string]string) (string, func()) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				fr := http2.NewFramer(c, c)
				pre := make([]byte, len(http2.ClientPreface))
				if _, err := c.Read(pre); err != nil {
					return
				}
				fr.WriteSettings()
				var buf strings.Builder
				_ = buf
				dec := hpack.NewDecoder(4096, nil)
				for {
					f, err := fr.ReadFrame()
					if err != nil {
						return
					}
					switch f := f.(type) {
					case *http2.SettingsFrame:
						if !f.IsAck() {
							fr.WriteSettingsAck()
						}
					case *http2.HeadersFrame:
						hdr := map[string]string{}
						fs, _ := dec.DecodeFull(f.HeaderBlockFragment())
						for _, h := range fs {
							hdr[h.Name] = h.Value
						}
						if got != nil {
							select {
							case got <- hdr:
							default:
							}
						}
						var hb strings.Builder
						_ = hb
						enc := newEnc()
						enc.field(":status", "200")
						enc.field("content-type", "application/grpc")
						enc.field("grpc-status", "13")
						enc.field("grpc-message", trailerMsg)
						fr.WriteHeaders(http2.HeadersFrameParam{StreamID: f.StreamID, BlockFragment: enc.bytes(), EndHeaders: true, EndStream: true})
					}
				}
			}()
		}
	}()
	return l.Addr().String(), func() { l.Close() }
}

type enc struct {
	e   *hpack.Encoder
	buf *bytesBuf
}
type bytesBuf struct{ b []byte }

func (b *bytesBuf) Write(p []byte) (int, error) { b.b = append(b.b, p...); return len(p), nil }
func newEnc() *enc {
	b := &bytesBuf{}
	return &enc{hpack.NewEncoder(b), b}
}
func (e *enc) field(n, v string) { e.e.WriteField(hpack.HeaderField{Name: n, Value: v}) }
func (e *enc) bytes() []byte     { return e.buf.b }

func dial(addr string) *grpc.ClientConn {
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		panic(err)
	}
	return cc
}

func call(cc *grpc.ClientConn, ctx context.Context) error {
	return cc.Invoke(ctx, "/p.S/M", &struct{}{}, &struct{}{}, grpc.ForceCodecV2(nopCodec{}))
}

func main() {
	if interopMain() {
		return
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.SplitN(sc.Text(), "\t", 2)
		switch f[0] {
		case "timeout":
			ns, _ := strconv.ParseInt(f[1], 10, 64)
			got := make(chan map[string]string, 4)
			addr, stop := rawServer("x", got)
			cc := dial(addr)
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ns))
			_ = call(cc, ctx)
			cancel()
			// the header on the wire depends on how far the deadline ran
			// down before the call was sent; the corpus takes the header
			// for ns itself, from grpc-go's encoder (encodeDuration, a
			// copy), after checking that the copy gives the wire's
			// header for the duration that header names
			select {
			case h := <-got:
				w := h["grpc-timeout"]
				d, ok := decodeTimeout(w)
				if !ok || encodeDuration(d) != w {
					fmt.Fprintf(os.Stderr, "grpcref: grpc-go sent grpc-timeout %q, which encodeDuration does not give back\n", w)
					os.Exit(1)
				}
			case <-time.After(2 * time.Second):
				// a deadline that ran out before the call was sent
			}
			fmt.Println(encodeDuration(time.Duration(ns)))
			cc.Close()
			stop()
		case "decode":
			addr, stop := rawServer(f[1], nil)
			cc := dial(addr)
			err := call(cc, context.Background())
			st, _ := status.FromError(err)
			fmt.Printf("%x\n", st.Message())
			cc.Close()
			stop()
		default:
			_ = codes.OK
			fmt.Println("?")
		}
	}
}

// encodeDuration is grpc-go v1.79.2's internal/grpcutil.EncodeDuration
// (Copyright 2020 gRPC authors, Apache License 2.0), copied because the
// package is internal.
func encodeDuration(t time.Duration) string {
	const maxTimeoutValue int64 = 100000000 - 1
	div := func(d, r time.Duration) int64 {
		if d%r > 0 {
			return int64(d/r + 1)
		}
		return int64(d / r)
	}
	if t <= 0 {
		return "0n"
	}
	if d := div(t, time.Nanosecond); d <= maxTimeoutValue {
		return strconv.FormatInt(d, 10) + "n"
	}
	if d := div(t, time.Microsecond); d <= maxTimeoutValue {
		return strconv.FormatInt(d, 10) + "u"
	}
	if d := div(t, time.Millisecond); d <= maxTimeoutValue {
		return strconv.FormatInt(d, 10) + "m"
	}
	if d := div(t, time.Second); d <= maxTimeoutValue {
		return strconv.FormatInt(d, 10) + "S"
	}
	if d := div(t, time.Minute); d <= maxTimeoutValue {
		return strconv.FormatInt(d, 10) + "M"
	}
	return strconv.FormatInt(div(t, time.Hour), 10) + "H"
}

// decodeTimeout reads a grpc-timeout header: digits and one unit.
func decodeTimeout(s string) (time.Duration, bool) {
	if len(s) < 2 {
		return 0, false
	}
	units := map[byte]time.Duration{'n': time.Nanosecond, 'u': time.Microsecond, 'm': time.Millisecond, 'S': time.Second, 'M': time.Minute, 'H': time.Hour}
	u, ok := units[s[len(s)-1]]
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(v) * u, true
}
