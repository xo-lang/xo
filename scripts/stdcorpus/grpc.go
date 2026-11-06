package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// grpcOracle runs internal/stdtest/grpcref (grpc-go observed on the wire) over the lines of in.
func grpcOracle(in string) []string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "grpcref")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	cmd.Stdin = strings.NewReader(in)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the grpc oracle (internal/stdtest/grpcref) failed: %v\n", err)
		os.Exit(1)
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

func grpcCorpus() string {
	r := rand.New(rand.NewSource(535 + protoFuzzSeed()))
	var in bytes.Buffer
	type tc struct{ ns int64 }
	var ts []tc
	// deadlines from a few microseconds to hours, so every unit is chosen
	for _, mag := range []int64{1e6, 1e8, 1e9, 1e10, 1e11, 1e12, 1e13} {
		for i := 0; i < 12; i++ {
			ns := mag/2 + r.Int63n(mag*5)
			ts = append(ts, tc{ns})
			fmt.Fprintf(&in, "timeout\t%d\n", ns)
		}
	}
	// escaped messages: valid, lower case hex, truncated and malformed escapes, high bytes
	var msgs []string
	alpha := []string{"a", "b", "c", " ", "%", "0", "1", "2", "9", "A", "B", "F", "a", "f", "z", "é"}
	for i := 0; i < 160; i++ {
		var b strings.Builder
		for n := r.Intn(12); n >= 0; n-- {
			if r.Intn(3) == 0 {
				b.WriteString(fmt.Sprintf("%%%02X", r.Intn(256)))
			} else {
				b.WriteString(alpha[r.Intn(len(alpha))])
			}
		}
		msgs = append(msgs, b.String())
	}
	for _, m := range msgs {
		fmt.Fprintf(&in, "decode\t%s\n", m)
	}
	out := grpcOracle(in.String())
	var o strings.Builder
	o.WriteString(header("grpc-go's client and server observed on the wire (internal/stdtest/grpcref): the grpc-timeout header a client sends and the status message a client decodes from grpc-message"))
	o.WriteString("use std/grpc\n\n")
	o.WriteString("/// grpc-timeout headers grpc-go sent (the deadline had run down a little by then: the check is that the header is\n/// what encode_timeout makes of its own decoding)\nconst TIMEOUTS: List[Str] = [\n")
	for i := range ts {
		// a deadline that ran out before the call was sent has no header
		if out[i] != "none" {
			fmt.Fprintf(&o, "  %s,\n", xq(out[i]))
		}
	}
	o.WriteString("]\n\n/// (grpc-message as sent, the status message grpc-go reports, in hex)\nconst MESSAGES: List[(Str, Str)] = [\n")
	for i, m := range msgs {
		got := out[len(ts)+i]
		if _, err := hex.DecodeString(got); err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "  (%s, %s),\n", xq(m), xq(got))
	}
	o.WriteString("]\n")
	o.WriteString(grpcCheckXo)
	return o.String()
}

const grpcCheckXo = `
test "grpc-timeout: the header grpc-go sends is what encode_timeout makes of its own decoding" {
  for h in TIMEOUTS {
    let ns = grpc.decode_timeout(h) ?? fault("cannot decode ${h}")
    if grpc.encode_timeout(ns) != h {
      fault("${h}: encode_timeout(decode_timeout) gives ${grpc.encode_timeout(ns)}")
    }
  }
}

test "grpc-message: decode_message answers what grpc-go's client reports" {
  for (esc, want) in MESSAGES {
    let got = grpc.decode_message_bytes(esc)
    if got.to_hex() != want {
      fault("${esc}: got ${got.to_hex()} want ${want}")
    }
  }
}

test "grpc: codes and frames" {
  expect(grpc.code_from(14)) == Some(grpc.Code.Unavailable)
  expect(grpc.code_from(17)) == None
  let (c, m, next) = grpc.unframe(grpc.frame(b"abc"), 0).or_fault("unframe")
  expect(c) == false
  expect(m) == b"abc"
  expect(next) == 8
}

test "grpc-message: the Str forms" {
  expect(grpc.decode_message("a%0Ab")) == "a\nb"
  expect(grpc.encode_message("a\nb")) == "a%0Ab"
}

test "grpc-message: encode then decode is the identity" {
  for (esc, _) in MESSAGES {
    let s = grpc.decode_message_bytes(esc)
    if grpc.decode_message_bytes(grpc.encode_message_bytes(s)) != s {
      fault("${esc}: round trip differs")
    }
  }
}
`
