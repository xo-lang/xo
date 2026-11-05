// Command amqporacle prints the std/amqp type system corpus: github.com/Azure/go-amqp v1.7.0
// (std/amqp's reference, decision 0521) encoding values as the body of a message and decoding
// them back. scripts/stdcorpus runs it; it is its own module so the repository's go.mod has no
// third-party requirement. go-amqp's encoding package is internal, so the cases go through the
// exported Message: MarshalBinary writes the value after the 3 byte descriptor of an AMQP value
// section, UnmarshalBinary reads it with ReadAny.
package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"

	amqp "github.com/Azure/go-amqp"
)

// show is the canonical text both sides print for a value: the type, then the value (strings and
// binary in hex, floats as their bits, maps sorted by their keys' text). go-amqp decodes a
// symbol as a string, so a symbol shows as str.
func show(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case bool:
		return fmt.Sprint(t)
	case uint8:
		return fmt.Sprintf("ubyte %d", t)
	case uint16:
		return fmt.Sprintf("ushort %d", t)
	case uint32:
		return fmt.Sprintf("uint %d", t)
	case uint64:
		return fmt.Sprintf("ulong %d", t)
	case int8:
		return fmt.Sprintf("byte %d", t)
	case int16:
		return fmt.Sprintf("short %d", t)
	case int32:
		return fmt.Sprintf("int %d", t)
	case int64:
		return fmt.Sprintf("long %d", t)
	case float32:
		return fmt.Sprintf("float %08x", math.Float32bits(t))
	case float64:
		return fmt.Sprintf("double %016x", math.Float64bits(t))
	case string:
		return "str " + hex.EncodeToString([]byte(t))
	case amqp.Symbol:
		return "str " + hex.EncodeToString([]byte(t))
	case []byte:
		return "bin " + hex.EncodeToString(t)
	case time.Time:
		return fmt.Sprintf("ts %d", t.UnixMilli())
	case amqp.UUID:
		return "uuid " + hex.EncodeToString(t[:])
	case []any:
		var parts []string
		for _, e := range t {
			parts = append(parts, show(e))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case map[any]any:
		var parts []string
		for k, e := range t {
			parts = append(parts, show(k)+"="+show(e))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}"
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Map {
		// go-amqp decodes a map whose keys are all strings as map[string]any
		var parts []string
		for _, k := range rv.MapKeys() {
			parts = append(parts, show(k.Interface())+"="+show(rv.MapIndex(k).Interface()))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}"
	}
	if rv.Kind() == reflect.Slice {
		var parts []string
		for i := 0; i < rv.Len(); i++ {
			parts = append(parts, show(rv.Index(i).Interface()))
		}
		return "array[" + strings.Join(parts, ",") + "]"
	}
	panic(fmt.Sprintf("show: %T", v))
}

type row struct {
	name  string
	bytes []byte
	shown string
}

func uuid(b byte) amqp.UUID {
	var u amqp.UUID
	for i := range u {
		u[i] = b + byte(i)
	}
	return u
}

func cases() []struct {
	name string
	v    any
} {
	type c = struct {
		name string
		v    any
	}
	var out []c
	add := func(name string, v any) { out = append(out, c{name, v}) }
	add("null", nil)
	add("true", true)
	add("false", false)
	for _, n := range []uint8{0, 1, 127, 128, 255} {
		add(fmt.Sprintf("ubyte %d", n), n)
	}
	for _, n := range []uint16{0, 1, 255, 256, 65535} {
		add(fmt.Sprintf("ushort %d", n), n)
	}
	for _, n := range []uint32{0, 1, 255, 256, 65535, 65536, 1<<32 - 1} {
		add(fmt.Sprintf("uint %d", n), n)
	}
	for _, n := range []uint64{0, 1, 255, 256, 1<<32 - 1, 1 << 32, 1<<63 - 1, 1 << 63, 1<<64 - 1} {
		add(fmt.Sprintf("ulong %d", n), n)
	}
	for _, n := range []int8{0, 1, -1, 127, -128} {
		add(fmt.Sprintf("byte %d", n), n)
	}
	for _, n := range []int16{0, 1, -1, 127, 128, -128, -129, 32767, -32768} {
		add(fmt.Sprintf("short %d", n), n)
	}
	for _, n := range []int32{0, 1, -1, 127, 128, -128, -129, math.MaxInt32, math.MinInt32} {
		add(fmt.Sprintf("int %d", n), n)
	}
	for _, n := range []int64{0, 1, -1, 127, 128, -128, -129, math.MaxInt32 + 1, math.MaxInt64, math.MinInt64} {
		add(fmt.Sprintf("long %d", n), n)
	}
	for _, f := range []float32{0, 1, -1.5, math.MaxFloat32, math.SmallestNonzeroFloat32, float32(math.Inf(1)), float32(math.Inf(-1)), float32(math.NaN())} {
		add(fmt.Sprintf("float %v", f), f)
	}
	for _, f := range []float64{0, 1, -1.5, math.MaxFloat64, math.SmallestNonzeroFloat64, math.Inf(1), math.Inf(-1), math.NaN(), 0.1} {
		add(fmt.Sprintf("double %v", f), f)
	}
	for _, n := range []int{0, 1, 2, 254, 255, 256, 257, 65535, 70000} {
		add(fmt.Sprintf("string len %d", n), strings.Repeat("é", n/2)+strings.Repeat("a", n%2))
		add(fmt.Sprintf("binary len %d", n), bytes.Repeat([]byte{0, 0xff, 7}, n/3+1)[:n])
		add(fmt.Sprintf("symbol len %d", n), amqp.Symbol(strings.Repeat("s", n)))
	}
	add("string unicode", "héllo, 世界 \U0001F600")
	for _, ms := range []int64{0, 1, -1, 1700000000123, -62135596800000, 253402300799999} {
		add(fmt.Sprintf("timestamp %d", ms), time.UnixMilli(ms).UTC())
	}
	add("uuid", uuid(1))
	add("empty list", []any{})
	add("list of scalars", []any{uint32(300), "hi", nil, true, int64(-5), []byte{1, 2}})
	add("nested list", []any{[]any{}, []any{[]any{uint8(1)}}, map[any]any{"k": "v"}})
	var big []any
	for i := 0; i < 300; i++ {
		big = append(big, uint32(i))
	}
	add("list of 300", big)
	add("empty map", map[any]any{})
	add("map one", map[any]any{"k": uint32(1)})
	add("map many", map[any]any{"a": uint32(1), "b": "two", amqp.Symbol("c"): []any{true}, int64(4): nil, uint8(5): 6.5})
	add("map nested", map[any]any{"outer": map[any]any{"inner": []any{int32(1), int32(2)}}})
	add("map string keys", map[string]any{"x": int64(1), "y": "z"})
	add("map symbol keys", map[amqp.Symbol]any{"x": int64(1), "y": "z"})
	// arrays: one constructor, the narrow form when every element fits
	add("array int8", []int8{1, -2, 3})
	add("array uint16", []uint16{1, 2, 65535})
	add("array int16", []int16{1, -2, 300})
	add("array uint32 narrow", []uint32{1, 2, 255})
	add("array uint32 wide", []uint32{1, 2, 256})
	add("array int32 narrow", []int32{1, -128, 127})
	add("array int32 wide", []int32{1, -129})
	add("array uint64 narrow", []uint64{0, 255})
	add("array uint64 wide", []uint64{0, 256, 1 << 63})
	add("array int64 narrow", []int64{-1, 1})
	add("array int64 wide", []int64{-1, 1 << 40})
	add("array float", []float32{1, 2.5})
	add("array double", []float64{1, 2.5})
	add("array bool", []bool{true, false, true})
	add("array string narrow", []string{"a", "", "bc"})
	add("array string wide", []string{"a", strings.Repeat("w", 300)})
	add("array symbol narrow", []amqp.Symbol{"a", "bc"})
	add("array symbol wide", []amqp.Symbol{"a", amqp.Symbol(strings.Repeat("w", 256))})
	add("array binary", [][]byte{{1}, {}, {2, 3}})
	add("array timestamp", []time.Time{time.UnixMilli(1), time.UnixMilli(-2)})
	add("array uuid", []amqp.UUID{uuid(1), uuid(9)})
	add("array map", []map[any]any{{"a": uint32(1)}, {}})
	var bigArr []uint32
	for i := 0; i < 100; i++ {
		bigArr = append(bigArr, uint32(1000+i))
	}
	add("array uint32 100 (array32)", bigArr)
	var bigStr []string
	for i := 0; i < 40; i++ {
		bigStr = append(bigStr, "item"+fmt.Sprint(i))
	}
	add("array string 40 (array32)", bigStr)
	return out
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "bench" {
		bench()
		return
	}
	var rows []row
	for _, c := range cases() {
		msg := &amqp.Message{Value: c.v}
		b, err := msg.MarshalBinary()
		if err != nil {
			panic(fmt.Sprintf("%s: %v", c.name, err))
		}
		if c.v == nil {
			// a message with no value has no body section
			rows = append(rows, row{c.name, []byte{0x40}, "null"})
			continue
		}
		if !bytes.HasPrefix(b, []byte{0x00, 0x53, 0x77}) {
			panic(fmt.Sprintf("%s: unexpected message prefix %x", c.name, b[:3]))
		}
		enc := b[3:]
		var back amqp.Message
		if err := back.UnmarshalBinary(b); err != nil {
			panic(fmt.Sprintf("%s: unmarshal: %v", c.name, err))
		}
		rows = append(rows, row{c.name, enc, show(back.Value)})
	}

	fmt.Println("// Generated by scripts/stdcorpus from the type system cases of github.com/Azure/go-amqp v1.7.0 (MIT, Copyright 2017 Kale Blankenship and Microsoft), its encoder's bytes and its decoder's values, against std/amqp (go1.26.1).")
	fmt.Println("// Do not edit: regenerate with go run ./scripts/stdcorpus.")
	fmt.Println()
	fmt.Println("use std/amqp")
	fmt.Println()
	fmt.Println(helpers)
	fmt.Println("/// (case, go-amqp's encoding in hex, go-amqp's decoded value shown)")
	fmt.Println("const CASES: List[(Str, Str, Str)] = [")
	for _, r := range rows {
		fmt.Printf("  (%q, \"%x\", %q),\n", r.name, r.bytes, r.shown)
	}
	fmt.Println("]")
	fmt.Println()
	fmt.Println(`/// (case, truncated length, go-amqp refuses it): every proper prefix of an encoding
const TRUNCATED: List[(Int, Int)] = [`)
	for i, r := range rows {
		// a few cut points: empty, 1, half, all but one
		for _, n := range cuts(len(r.bytes)) {
			msg := append([]byte{0x00, 0x53, 0x77}, r.bytes[:n]...)
			var m amqp.Message
			err := m.UnmarshalBinary(msg)
			if err == nil {
				// go-amqp accepts this prefix (a shorter value followed by nothing)
				continue
			}
			fmt.Printf("  (%d, %d),\n", i, n)
		}
	}
	fmt.Println("]")
	fmt.Println()
	fmt.Println(tests)
	fmt.Println()
	fmt.Println("/// (case, the frame go-amqp's client wrote, in hex): capture over a fake peer")
	fmt.Println("const FRAMES: List[(Str, Str)] = [")
	for _, r := range clientFrames() {
		fmt.Printf("  (%q, \"%x\"),\n", r.name, r.raw)
	}
	fmt.Println("]")
	fmt.Println()
	fmt.Println(frameTests)
}

const frameTests = `fn kind(b: amqp.FrameBody) -> Str {
  match b {
    Open(_) => "open"
    Begin(_) => "begin"
    Attach(_) => "attach"
    Flow(_) => "flow"
    Transfer(_) => "transfer"
    Disposition(_) => "disposition"
    Detach(_) => "detach"
    End(_) => "end"
    Close(_) => "close"
    SaslInit(_) => "sasl-init"
    Empty => "empty"
    _ => "other"
  }
}

test "frames go-amqp's client wrote decode and encode back to the same bytes" {
  for (name, hex) in FRAMES {
    let b = Bytes.from_hex(hex).or_fault("hex")
    let (f, n) = amqp.decode_frame(b).or_fault(name)
    expect(n) == b.len()
    expect(name.ends_with(kind(f.body))) == true
    expect(amqp.encode_frame(f).or_fault(name)) == b
  }
}
`

func cuts(n int) []int {
	set := map[int]bool{}
	for _, c := range []int{0, 1, 2, 3, n / 2, n - 2, n - 1} {
		if c >= 0 && c < n {
			set[c] = true
		}
	}
	var out []int
	for c := range set {
		out = append(out, c)
	}
	sort.Ints(out)
	return out
}

const helpers = `fn hexn(v: Int, digits: Int) -> Str {
  var s = ""
  var i = digits - 1
  while i >= 0 {
    let d = (v >> (4 * i)) & 15
    s += "0123456789abcdef"[d..d + 1]
    i -= 1
  }
  s
}

/// The canonical text of a value (the same as the oracle's): symbols show as strings, as
/// go-amqp decodes them; maps are sorted by their keys' text.
fn show(v: amqp.Value) -> Str {
  match v {
    Null => "null"
    Bool(b) => "${b}"
    UByte(x) => "ubyte ${x}"
    UShort(x) => "ushort ${x}"
    UInt(x) => "uint ${x}"
    ULong(x) => "ulong ${x}"
    Byte(x) => "byte ${x}"
    Short(x) => "short ${x}"
    Int(x) => "int ${x}"
    Long(x) => "long ${x}"
    Float(bits) => "float ${hexn(Int(bits), 8)}"
    Double(x) => "double ${hexn(x.to_bits(), 16)}"
    Decimal32(b) => "decimal32 ${hexn(Int(b), 8)}"
    Decimal64(b) => "decimal64 ${hexn(b.to_bits(), 16)}"
    Decimal128(b) => "decimal128 ${b.to_hex()}"
    Char(r) => "char ${Int(r)}"
    Timestamp(ms) => "ts ${ms}"
    Uuid(b) => "uuid ${b.to_hex()}"
    Binary(b) => "bin ${b.to_hex()}"
    Str(s) => "str ${s.bytes().to_hex()}"
    Symbol(s) => "str ${s.bytes().to_hex()}"
    List(items) => "[" + items.map(fn(x) { show(x) }).join(",") + "]"
    Map(pairs) => "{" + pairs.map(fn(p) { show(p.0) + "=" + show(p.1) }).sort().join(",") + "}"
    Array(items) => "array[" + items.map(fn(x) { show(x) }).join(",") + "]"
    Described(d, x) => "described " + show(d) + " " + show(x)
  }
}
`

const tests = `test "go-amqp's encodings decode to go-amqp's values and encode back to the same bytes" {
  for (name, hex, shown) in CASES {
    let b = Bytes.from_hex(hex).or_fault("hex")
    let (v, n) = amqp.decode(b).or_fault(name)
    expect(n) == b.len()
    expect(show(v)) == shown
    expect(amqp.encode(v).or_fault(name)) == b
  }
}

test "a truncated encoding is refused" {
  for (i, n) in TRUNCATED {
    let b = Bytes.from_hex(CASES[i].1).or_fault("hex")
    expect(amqp.decode(b[0..n]).is_err()) == true
  }
}
`

// bench times go-amqp's MarshalBinary and UnmarshalBinary over every case 2000 times (the timing row of
// std/changes/amqp.md; the Xo side decodes and encodes the same encodings).
func bench() {
	var msgs [][]byte
	for _, c := range cases() {
		if c.v == nil {
			continue
		}
		b, err := (&amqp.Message{Value: c.v}).MarshalBinary()
		if err != nil {
			panic(err)
		}
		msgs = append(msgs, b)
	}
	t0 := time.Now()
	n := 0
	for i := 0; i < 2000; i++ {
		for _, b := range msgs {
			var m amqp.Message
			if err := m.UnmarshalBinary(b); err != nil {
				panic(err)
			}
			out, err := m.MarshalBinary()
			if err != nil {
				panic(err)
			}
			n += len(out)
		}
	}
	fmt.Printf("go decode+encode of %d cases x 2000: %v (%d)\n", len(msgs), time.Since(t0), n)
}
