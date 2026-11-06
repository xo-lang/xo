// Command protoref is the differential oracle for std/protobuf (decision 0530): google.golang.org/protobuf's protowire
// (and, as the module grows, its proto package) over the inputs scripts/stdcorpus sends. One line in per case, "OP hex
// [args]", one JSON line out.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protowire"
)

type ans struct {
	Err string `json:"err,omitempty"`
	V   string `json:"v,omitempty"`
	N   int    `json:"n"`
}

func kind(n int) string {
	switch {
	case n == -1:
		return "Truncated"
	case n == -2:
		return "BadTag"
	case n == -3:
		return "Overflow"
	case n == -4:
		return "BadWire"
	case n == -5:
		return "EndGroup"
	case n == -6:
		return "Recursion"
	}
	return fmt.Sprintf("code%d", n)
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		op := f[0]
		var b []byte
		if len(f) > 1 && f[1] != "-" {
			b, _ = hex.DecodeString(f[1])
		}
		var a ans
		switch op {
		case "varint":
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(v, 10), N: n}
			}
		case "tag":
			num, typ, n := protowire.ConsumeTag(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: fmt.Sprintf("%d %d", num, typ), N: n}
			}
		case "fixed32":
			v, n := protowire.ConsumeFixed32(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(uint64(v), 10), N: n}
			}
		case "fixed64":
			v, n := protowire.ConsumeFixed64(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: strconv.FormatUint(v, 10), N: n}
			}
		case "bytes":
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				a = ans{Err: kind(n)}
			} else {
				a = ans{V: hex.EncodeToString(v), N: n}
			}
		case "field":
			// the tag, then the whole field value
			num, typ, n := protowire.ConsumeTag(b)
			if n < 0 {
				a = ans{Err: kind(n)}
				break
			}
			m := protowire.ConsumeFieldValue(num, typ, b[n:])
			if m < 0 {
				a = ans{Err: kind(m)}
			} else {
				a = ans{V: fmt.Sprintf("%d %d", num, typ), N: n + m}
			}
		case "encvarint":
			u, _ := strconv.ParseUint(f[1], 10, 64)
			a = ans{V: hex.EncodeToString(protowire.AppendVarint(nil, u)), N: protowire.SizeVarint(u)}
		case "zigzag":
			i, _ := strconv.ParseInt(f[1], 10, 64)
			z := protowire.EncodeZigZag(i)
			a = ans{V: strconv.FormatUint(z, 10), N: int(protowire.DecodeZigZag(z))}
		default:
			a = ans{Err: "unknown op " + op}
		}
		enc.Encode(a)
	}
}
