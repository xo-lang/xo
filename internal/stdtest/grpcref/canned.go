package main

// The canned modes, for internal/stdtest TestGrpcFuzz (task #115):
//
//	cannedserve FILE   a raw HTTP/2 server: the call whose x-case metadata is N gets case N of FILE (a JSON object a
//	                   line: the response headers, the body's bytes cut into DATA frames, and the trailers, or none
//	                   for a trailers-only response); prints its address
//	cannedclient ADDR N  grpc-go's client makes calls 0 to N-1 (a stream: CloseSend, then RecvMsg to the end) and
//	                   prints for each "== ", "m HEX" per message received, "s CODE" and "x HEX" (the status's code
//	                   and the hex of its message)

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
	"google.golang.org/grpc"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type cannedCase struct {
	H     [][2]string `json:"h"`
	Body  string      `json:"body"`
	Split []int       `json:"split"`
	T     [][2]string `json:"t"`
}

func cannedServe(file string) {
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	var cases []cannedCase
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		var c cannedCase
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			panic(err)
		}
		cases = append(cases, c)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	fmt.Println(l.Addr().String())
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go cannedConn(c, cases)
	}
}

func cannedConn(c net.Conn, cases []cannedCase) {
	defer c.Close()
	pre := make([]byte, len(http2.ClientPreface))
	if _, err := ioReadFull(c, pre); err != nil {
		return
	}
	fr := http2.NewFramer(c, c)
	fr.SetMaxReadFrameSize(1 << 24)
	fr.WriteSettings(http2.Setting{ID: http2.SettingMaxFrameSize, Val: 1 << 24})
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
		case *http2.PingFrame:
			if !f.IsAck() {
				fr.WritePing(true, f.Data)
			}
		case *http2.HeadersFrame:
			n := -1
			fs, _ := dec.DecodeFull(f.HeaderBlockFragment())
			for _, h := range fs {
				if h.Name == "x-case" {
					n, _ = strconv.Atoi(h.Value)
				}
			}
			if n < 0 || n >= len(cases) {
				return
			}
			k := cases[n]
			id := f.StreamID
			e := newEnc()
			for _, h := range k.H {
				e.field(h[0], h[1])
			}
			fr.WriteHeaders(http2.HeadersFrameParam{StreamID: id, BlockFragment: e.bytes(), EndHeaders: true, EndStream: k.T == nil})
			if k.T == nil {
				continue
			}
			body, _ := hex.DecodeString(k.Body)
			for i := 0; len(body) > 0; i++ {
				m := len(body)
				if i < len(k.Split) && k.Split[i] > 0 && k.Split[i] < m {
					m = k.Split[i]
				}
				fr.WriteData(id, false, body[:m])
				body = body[m:]
			}
			e = newEnc()
			for _, h := range k.T {
				e.field(h[0], h[1])
			}
			fr.WriteHeaders(http2.HeadersFrameParam{StreamID: id, BlockFragment: e.bytes(), EndHeaders: true, EndStream: true})
		}
	}
}

func ioReadFull(c net.Conn, b []byte) (int, error) {
	n := 0
	for n < len(b) {
		m, err := c.Read(b[n:])
		n += m
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

func cannedClient(addr string, n int) {
	cc := dial(addr)
	defer cc.Close()
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	desc := &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}
	for i := 0; i < n; i++ {
		fmt.Fprintln(w, "== ")
		ctx := metadata.AppendToOutgoingContext(context.Background(), "x-case", strconv.Itoa(i))
		st, err := cc.NewStream(ctx, desc, "/f.S/M", grpc.ForceCodecV2(bytesCodec{}))
		if err == nil {
			st.CloseSend()
			for {
				var b []byte
				if err = st.RecvMsg(&b); err != nil {
					break
				}
				fmt.Fprintf(w, "m %x\n", b)
			}
		}
		if err == io.EOF {
			// the end of an OK stream
			err = nil
		}
		s, _ := status.FromError(err)
		fmt.Fprintf(w, "s %d\nx %x\n", s.Code(), s.Message())
	}
}
