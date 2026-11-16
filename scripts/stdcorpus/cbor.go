package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// cborRef is what the oracle (internal/stdtest/cborref, github.com/
// fxamacker/cbor/v2) makes of one input.
type cborRef struct {
	WF    bool   `json:"wf"`
	Diag  string `json:"diag"`
	Canon string `json:"canon"`
	Dup   string `json:"dup"`
}

// cborOracle runs the oracle over the inputs (hex a line).
func cborOracle(inputs [][]byte) []cborRef {
	return cborOracleRun(inputs)
}

// cborTypedRef is what the oracle makes of one input decoded into the typed
// struct and marshaled again (decision 0527).
type cborTypedRef struct {
	Err bool   `json:"err"`
	Out string `json:"out"`
}

func cborTypedOracle(mode string, inputs [][]byte) []cborTypedRef {
	var in bytes.Buffer
	for _, b := range inputs {
		in.WriteString(hex.EncodeToString(b))
		in.WriteByte('\n')
	}
	cmd := exec.Command("go", "run", ".", mode)
	cmd.Dir = filepath.Join("internal", "stdtest", "cborref")
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the cbor oracle (typed) failed: %v\n", err)
		os.Exit(1)
	}
	var refs []cborTypedRef
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var r cborTypedRef
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			panic(err)
		}
		refs = append(refs, r)
	}
	if len(refs) != len(inputs) {
		panic("cbor typed oracle: wrong number of answers")
	}
	return refs
}

func cborOracleRun(inputs [][]byte) []cborRef {
	var in bytes.Buffer
	for _, b := range inputs {
		in.WriteString(hex.EncodeToString(b))
		in.WriteByte('\n')
	}
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "cborref")
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the cbor oracle (internal/stdtest/cborref) failed: %v\n", err)
		os.Exit(1)
	}
	var refs []cborRef
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		var r cborRef
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			panic(err)
		}
		refs = append(refs, r)
	}
	if len(refs) != len(inputs) {
		panic("cbor oracle: wrong number of answers")
	}
	return refs
}

// RFC 8949 Appendix A, the encodings of its examples (the decoded column is
// what the oracle says, which the tests compare against).
var cborRFC = []string{
	"00", "01", "0a", "17", "1818", "1819", "1864", "1903e8", "1a000f4240", "1b000000e8d4a51000",
	"1bffffffffffffffff", "c249010000000000000000", "3bffffffffffffffff", "c349010000000000000000",
	"20", "29", "3863", "3903e7", "f90000", "f98000", "f93c00", "fb3ff199999999999a", "f93e00", "f97bff",
	"fa47c35000", "fa7f7fffff", "fb7e37e43c8800759c", "f90001", "f90400", "f9c400", "fbc010666666666666",
	"f97c00", "f97e00", "f9fc00", "fa7f800000", "fa7fc00000", "faff800000", "fb7ff0000000000000",
	"fb7ff8000000000000", "fbfff0000000000000", "f4", "f5", "f6", "f7", "f0", "f8ff",
	"c074323031332d30332d32315432303a30343a30305a", "c11a514b67b0", "c1fb41d452d9ec200000",
	"d74401020304", "d818456449455446", "d82076687474703a2f2f7777772e6578616d706c652e636f6d",
	"40", "4401020304", "60", "6161", "6449455446", "62225c", "62c3bc", "63e6b0b4", "64f0908591",
	"80", "83010203", "8301820203820405",
	"98190102030405060708090a0b0c0d0e0f101112131415161718181819",
	"a0", "a201020304", "a26161016162820203", "826161a161626163",
	"a56161614161626142616361436164614461656145",
	"5f42010243030405ff", "7f657374726561646d696e67ff", "9fff", "9f018202039f0405ffff",
	"9f01820203820405ff", "83018202039f0405ff", "83019f0203ff820405",
	"9f0102030405060708090a0b0c0d0e0f101112131415161718181819ff",
	"bf61610161629f0203ffff", "826161bf61626163ff", "bf6346756ef563416d7421ff",
}

// Curated cases: malformed heads, truncations, reserved values, strings,
// tags, nesting, repeated keys, map orders.
var cborCurated = []string{
	"", "ff", "1c", "1d", "1e", "1f", "3c", "3f", "5c", "7f", "7f41", "7fff", "7f6161ff", "7f7fff", "5f6161ff", "5f5fff", "5f4100ff", "5f40ff",
	"9f", "9f01", "9f01ff", "bf", "bf01", "bf01ff", "bf0102ff", "bfff", "a1", "a101", "a1ff", "9a00000002", "9a00ffffff",
	"9b0000000100000000", "bb7fffffffffffffff", "5b7fffffffffffffff", "5bffffffffffffffff", "7b7fffffffffffffff", "4a", "58", "5801", "590001", "5a00000001",
	"1801", "1900ff", "1a0000ffff", "1b00000000ffffffff", "f800", "f81f", "f820", "f818", "f819", "f8", "f9", "f97c", "fa7f80", "fb7ff0",
	"f8f8", "f8ff00", "62ffff", "61ff", "62c328", "63e0a080", "64f0908591", "63ed a080"[0:2] + "a080", "66edbfbfedbfbf",
	"7f62c3ff62bcff", "7f62c3bcff", "6120", "6100", "617f", "6180", "62c200", "62c0af", "c0", "c1", "c0c0", "c0c0c0f6", "c1c1c1c1c100", "dbffffffffffffffff00",
	"d8", "d80100", "d9ffff00", "db000000000000000100", "c2", "c240", "c2c2c240", "c3c340",
	"a2616101616102", "a201020102", "a2f93c0001fb3ff000000000000002", "a2f97e0001f97e0002", "a2f90000010000f9800002",
	"a3616101616202616101", "a201010101", "a1a10101a10101", "a180a280a280", "a280018001",
	"a50a01186402202603617a04626161058161", "a60a01186402202603617a04626161058161f4f5", "a4f4016161021863030a", "a7f6f5f4f7f0f8ff00",
	"a2f40100", "a2182a01182a02", "a2420102014201020202"[0:12] + "02", "a3f8ff01f0020a03",
	"8180", "818180", "81818181818181818181", "9f9f9f9f9f9f9f9fffffffffffffffff", "9f9f9f9f9f9f9f9f", "bf6161bf6162bf6163ffffff",
	"c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c0c000",
	"1800", "1817", "190000", "190017", "1a00000000", "1b0000000000000000", "3800", "3817", "380000"[0:6],
	"f93c00ff", "00ff", "0000", "8301020304"[0:8] + "04", "f4f4", "f6f7", "40ff",
	"fa00000001", "fa007fffff", "fa00800000", "fa33800000", "fa38800000", "fa387fc000", "fb3e70000000000000", "fb3e60000000000000", "fb3f10000000000000",
	"fb3810000000000000", "fb380fffffffffffff", "fb47efffffe0000000", "fb47effffff0000000", "fb7fefffffffffffff", "fb0000000000000001", "fb8000000000000000",
	"f97bfe", "f97c01", "f97e01", "f9ffff", "f903ff", "f90400", "f90200", "f9fbff", "fa7f7fffff", "fa7f800001", "fa7fffffff",
	"fb3ff0000000000001", "fb4000000000000000", "fb40240000000000", "fb4059000000000000"[0:18] + "00",
}

func cborMutate(r *rand.Rand, b []byte) []byte {
	out := append([]byte(nil), b...)
	switch r.Intn(6) {
	case 0:
		if len(out) > 0 {
			out[r.Intn(len(out))] ^= byte(1 << r.Intn(8))
		}
	case 1:
		if len(out) > 0 {
			out = out[:r.Intn(len(out))]
		}
	case 2:
		i := r.Intn(len(out) + 1)
		out = append(out[:i], append([]byte{byte(r.Intn(256))}, out[i:]...)...)
	case 3:
		if len(out) > 0 {
			i := r.Intn(len(out))
			out = append(out[:i], out[i+1:]...)
		}
	case 4:
		out = append(out, byte(r.Intn(256)))
	default:
		if len(out) > 0 {
			out[r.Intn(len(out))] = byte(r.Intn(256))
		}
	}
	return out
}

// ---- derive Cbor (decision 0527): decode into a struct and marshal it again ----

func cbHead(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 1<<8:
		return []byte{major<<5 | 24, byte(n)}
	case n < 1<<16:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	case n < 1<<32:
		return []byte{major<<5 | 26, byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
	b := []byte{major<<5 | 27}
	for i := 7; i >= 0; i-- {
		b = append(b, byte(n>>(8*uint(i))))
	}
	return b
}

func cbInt(n int64) []byte {
	if n >= 0 {
		return cbHead(0, uint64(n))
	}
	return cbHead(1, uint64(-1-n))
}
func cbText(s string) []byte  { return append(cbHead(3, uint64(len(s))), s...) }
func cbBytes(s string) []byte { return append(cbHead(2, uint64(len(s))), s...) }
func cbArr(items ...[]byte) []byte {
	b := cbHead(4, uint64(len(items)))
	for _, it := range items {
		b = append(b, it...)
	}
	return b
}
func cbMap(kv ...[]byte) []byte {
	b := cbHead(5, uint64(len(kv)/2))
	for _, it := range kv {
		b = append(b, it...)
	}
	return b
}

// cbValue is a random item: scalars of every kind, small arrays and maps,
// tags, null.
func cbValue(r *rand.Rand, depth int) []byte {
	n := 11
	if depth >= 2 {
		n = 8
	}
	switch r.Intn(n) {
	case 0:
		return cbInt(int64(r.Intn(40)) - 5)
	case 1:
		return cbInt(r.Int63() >> uint(r.Intn(62)))
	case 2:
		return cbText([]string{"", "a", "Ann", "z\u00e9"}[r.Intn(4)])
	case 3:
		return cbBytes([]string{"", "ab", "\x00\xff"}[r.Intn(3)])
	case 4:
		return []byte{0xf4 + byte(r.Intn(2))}
	case 5:
		return []byte{0xf6}
	case 6:
		return []byte{0xfb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0} // 1.5
	case 7:
		return []byte{0xf9, 0x3c, 0x00} // 1.0 as a half
	case 8:
		var items [][]byte
		for i := r.Intn(4); i > 0; i-- {
			items = append(items, cbValue(r, depth+1))
		}
		return cbArr(items...)
	case 9:
		var kv [][]byte
		for i := r.Intn(3); i > 0; i-- {
			kv = append(kv, cbText([]string{"a", "b", "c"}[r.Intn(3)]), cbValue(r, depth+1))
		}
		return cbMap(kv...)
	}
	return append(cbHead(6, uint64(r.Intn(3)+1)), cbValue(r, depth+1)...)
}

// cbFor is a value of the kind the field wants, most of the time.
func cbFor(r *rand.Rand, kind string) []byte {
	if r.Intn(8) == 0 {
		if kind == "time" {
			return []byte{0xf5}
		}
		return cbValue(r, 1)
	}
	switch kind {
	case "text":
		return cbText([]string{"", "a", "Ann", "z\u00e9"}[r.Intn(4)])
	case "int":
		return cbInt(int64(r.Intn(2000)) - 500)
	case "float":
		if r.Intn(3) == 0 {
			return cbInt(int64(r.Intn(10)))
		}
		return []byte{0xfb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}
	case "bool":
		return []byte{0xf4 + byte(r.Intn(2))}
	case "time":
		switch r.Intn(6) {
		case 0:
			return []byte{0xfb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}
		case 1:
			return append(cbHead(6, 1), cbInt(int64(r.Intn(2000000000)))...)
		case 2:
			return append(cbHead(6, 0), cbText("2020-01-02T03:04:05Z")...)
		case 3:
			return cbText("2021-12-31T23:59:59+01:00")
		case 4:
			return cbInt(int64(r.Intn(2000000000)) - 1000000000)
		}
		return cbInt(int64(r.Intn(100000)))
	case "bytes":
		if r.Intn(4) == 0 {
			return cbArr(cbInt(1), cbInt(int64(r.Intn(256))))
		}
		return cbBytes([]string{"", "ab", "\x00\xff"}[r.Intn(3)])
	case "tags":
		var items [][]byte
		for i := r.Intn(4); i > 0; i-- {
			items = append(items, cbFor(r, "text"))
		}
		return cbArr(items...)
	case "attrs":
		var kv [][]byte
		for i := r.Intn(3); i > 0; i-- {
			kv = append(kv, cbText([]string{"a", "b", "c"}[r.Intn(3)]), cbFor(r, "int"))
		}
		return cbMap(kv...)
	}
	return cbValue(r, 1)
}

func cbAddr(r *rand.Rand) []byte {
	if r.Intn(5) == 0 {
		return cbValue(r, 1)
	}
	var kv [][]byte
	for _, k := range []string{"city", "zip", "City", "other"} {
		if r.Intn(2) == 0 {
			kind := "text"
			if k == "zip" {
				kind = "int"
			}
			kv = append(kv, cbText(k), cbFor(r, kind))
		}
	}
	return cbMap(kv...)
}

func cborTypedInputs(r *rand.Rand, n int) [][]byte {
	keys := []string{"name", "Name", "NAME", "tags", "addr", "ratio", "ok", "raw", "attrs", "zzz", "skip", "Skip"}
	var out [][]byte
	for i := 0; i < n; i++ {
		var kv [][]byte
		for _, k := range keys {
			if r.Intn(3) != 0 {
				continue
			}
			var v []byte
			switch k {
			case "name", "Name", "NAME", "skip", "Skip":
				v = cbFor(r, "text")
			case "tags":
				v = cbFor(r, "tags")
			case "addr":
				v = cbAddr(r)
			case "ratio":
				v = cbFor(r, "float")
			case "ok":
				v = cbFor(r, "bool")
			case "raw":
				v = cbFor(r, "bytes")
			case "attrs":
				v = cbFor(r, "attrs")
			default:
				v = cbValue(r, 0)
			}
			kv = append(kv, cbText(k), v)
		}
		if r.Intn(3) == 0 {
			kv = append(kv, cbInt(7), cbFor(r, "int"))
		}
		if r.Intn(8) == 0 {
			kv = append(kv, cbText("7"), cbValue(r, 1))
		}
		// shuffle the pairs
		for j := len(kv)/2 - 1; j > 0; j-- {
			k := r.Intn(j + 1)
			kv[2*j], kv[2*k] = kv[2*k], kv[2*j]
			kv[2*j+1], kv[2*k+1] = kv[2*k+1], kv[2*j+1]
		}
		in := cbMap(kv...)
		switch r.Intn(12) {
		case 0:
			in = cbValue(r, 0)
		case 1:
			in = cborMutate(r, in)
		}
		out = append(out, in)
	}
	return out
}

func cbAddrs(r *rand.Rand) []byte {
	var items [][]byte
	for i := r.Intn(4); i > 0; i-- {
		items = append(items, cbAddr(r))
	}
	return cbArr(items...)
}

func cbPerson(r *rand.Rand) []byte {
	in := cborTypedInputs(r, 1)[0]
	return in
}

func cborDeepInputs(r *rand.Rand, n int) [][]byte {
	var out [][]byte
	for i := 0; i < n; i++ {
		var kv [][]byte
		add := func(k string, v []byte) { kv = append(kv, cbText(k), v) }
		if r.Intn(2) == 0 {
			if r.Intn(4) == 0 {
				add("addrs", cbValue(r, 1))
			} else {
				add("addrs", cbAddrs(r))
			}
		}
		if r.Intn(2) == 0 {
			if r.Intn(4) == 0 {
				add("by", cbValue(r, 1))
			} else {
				var m [][]byte
				for j := r.Intn(3); j > 0; j-- {
					m = append(m, cbText([]string{"p", "q", "r"}[r.Intn(3)]), cbAddr(r))
				}
				add("by", cbMap(m...))
			}
		}
		if r.Intn(2) == 0 {
			add("opt", cbFor(r, "int"))
		}
		if r.Intn(2) == 0 {
			if r.Intn(4) == 0 {
				add("grid", cbValue(r, 1))
			} else {
				var rows [][]byte
				for j := r.Intn(3); j > 0; j-- {
					var row [][]byte
					for k := r.Intn(3); k > 0; k-- {
						row = append(row, cbFor(r, "int"))
					}
					rows = append(rows, cbArr(row...))
				}
				add("grid", cbArr(rows...))
			}
		}
		if r.Intn(3) == 0 {
			add("maybe", cbFor(r, "float"))
		}
		if r.Intn(3) == 0 {
			add("flag", cbFor(r, "bool"))
		}
		if r.Intn(3) == 0 {
			if r.Intn(2) == 0 {
				add("nested", cbValue(r, 1))
			} else {
				add("nested", cbPerson(r))
			}
		}
		if r.Intn(3) == 0 {
			add("count", cbFor(r, "int"))
		}
		if r.Intn(2) == 0 {
			add("when", cbFor(r, "time"))
		}
		if r.Intn(3) == 0 {
			add("dur", cbFor(r, "int"))
		}
		if r.Intn(3) == 0 {
			add("spent", cbFor(r, "int"))
		}
		in := cbMap(kv...)
		if r.Intn(12) == 0 {
			in = cborMutate(r, in)
		}
		out = append(out, in)
	}
	return out
}

func cborPairInputs(r *rand.Rand, n int) [][]byte {
	var out [][]byte
	for i := 0; i < n; i++ {
		items := [][]byte{cbFor(r, "int"), cbFor(r, "text"), cbFor(r, "int"), cbFor(r, "tags")}
		if r.Intn(3) == 0 {
			items[2] = []byte{0xf6}
		}
		switch r.Intn(10) {
		case 0:
			items = items[:r.Intn(4)]
		case 1:
			items = append(items, cbFor(r, "int"))
		}
		in := cbArr(items...)
		switch r.Intn(15) {
		case 0:
			in = cbValue(r, 0)
		case 1:
			in = cborMutate(r, in)
		}
		out = append(out, in)
	}
	return out
}

const cborTypedXo = `
struct Addr {
  city: Str
  zip: Int
} derive Cbor

struct Person {
  name: Str
  age cbor "7,keyasint": Int
  tags as "tags,omitempty": List[Str]
  addr: Addr?
  ratio: Float
  ok: Bool
  raw: Bytes
  attrs: Map[Str, Int]
  skip as "-": Str
} derive Cbor

test "unmarshal_prefix leaves what follows, strict refuses an unknown key" {
  let p = Person{name: "Ann", age: 3, tags: [], addr: None, ratio: 1.5, ok: true, raw: b"", attrs: {}, skip: ""}
  let b = cbor.marshal(p).ok() ?? fault("marshal")
  match cbor.unmarshal_prefix[Person](b + b"\x01") {
    Ok((q, n)) => {
      expect(q.name) == "Ann"
      expect(n) == b.len()
    }
    Err(e) => fault(e.display())
  }
  expect(cbor.unmarshal[Person](b + b"\x01").is_err()) == true
  let extra = b"\xa2\x64name\x63Ann\x61z\x01"
  expect(cbor.unmarshal[Person](extra).is_err()) == false
  expect(cbor.unmarshal[Person](extra, strict: true).is_err()) == true
}

struct Pair {
  a cbor ",toarray": Int
  b: Str
  c: Int?
  d: List[Str]
} derive Cbor

test "toarray structs agree with the oracle" {
  var bad: List[Str] = []
  for (data, err, out) in TYPED3 {
    match cbor.unmarshal[Pair](data) {
      Ok(p) => {
        if err {
          bad.push("accepted " + data.to_hex())
        } else {
          match cbor.marshal(p) {
            Ok(b) => {
              if b.to_hex() != out {
                bad.push("marshal " + data.to_hex() + ": " + b.to_hex() + " want " + out)
              }
            }
            Err(e) => bad.push("marshal failed " + data.to_hex() + ": " + e.display())
          }
        }
      }
      Err(_) => {
        if !err {
          bad.push("refused " + data.to_hex())
        }
      }
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

struct Deep {
  addrs: List[Addr]
  by: Map[Str, Addr]
  opt: Int?
  grid: List[List[Int]]
  maybe: Float?
  flag: Bool?
  nested: Person?
  count as "count,omitempty": Int
  when: Time?
  dur: Duration?
  spent as "spent,omitempty": Duration
} derive Cbor

test "unmarshal and marshal into nested types agree with the oracle" {
  var bad: List[Str] = []
  for (data, err, out) in TYPED2 {
    match cbor.unmarshal[Deep](data) {
      Ok(p) => {
        if err {
          bad.push("accepted " + data.to_hex())
        } else {
          match cbor.marshal(p) {
            Ok(b) => {
              if b.to_hex() != out {
                bad.push("marshal " + data.to_hex() + ": " + b.to_hex() + " want " + out)
              }
            }
            Err(e) => bad.push("marshal failed " + data.to_hex() + ": " + e.display())
          }
        }
      }
      Err(_) => {
        if !err {
          bad.push("refused " + data.to_hex())
        }
      }
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}

test "unmarshal and marshal into a struct agree with the oracle" {
  var bad: List[Str] = []
  for (data, err, out) in TYPED {
    match cbor.unmarshal[Person](data) {
      Ok(p) => {
        if err {
          bad.push("accepted " + data.to_hex())
        } else {
          match cbor.marshal(p) {
            Ok(b) => {
              if b.to_hex() != out {
                bad.push("marshal " + data.to_hex() + ": " + b.to_hex() + " want " + out)
              }
            }
            Err(e) => bad.push("marshal failed " + data.to_hex() + ": " + e.display())
          }
        }
      }
      Err(_) => {
        if !err {
          bad.push("refused " + data.to_hex())
        }
      }
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`

func cborCorpus() string {
	var inputs [][]byte
	add := func(h string) {
		b, err := hex.DecodeString(strings.ReplaceAll(h, " ", ""))
		if err != nil {
			panic(fmt.Sprintf("cbor corpus: bad hex %q", h))
		}
		inputs = append(inputs, b)
	}
	for _, h := range cborRFC {
		add(h)
	}
	for _, h := range cborCurated {
		add(h)
	}
	r := rand.New(rand.NewSource(42))
	base := len(inputs)
	for i := 0; i < base; i++ {
		inputs = append(inputs, cborMutate(r, inputs[i]))
	}
	refs := cborOracle(inputs)

	var o strings.Builder
	o.WriteString(header("RFC 8949 CBOR: decode, well_formed, diagnostic and the deterministic encode, on the Appendix A examples, malformed and edge inputs, and changed copies of them, against github.com/fxamacker/cbor/v2 (MIT) run by internal/stdtest/cborref"))
	o.WriteString("use std/big\nuse std/cbor\nuse std/json\n\n")
	o.WriteString("/// (CBOR bytes, well formed, diagnostic notation or ERR, UTF8 or SKIP, deterministic encoding in hex or ERR or SKIP, repeated map key: yes, no or ?)\nconst CASES: List[(Bytes, Bool, Str, Str, Str)] = [\n")
	for i, in := range inputs {
		rf := refs[i]
		fmt.Fprintf(&o, "  (%s, %v, %s, %s, %s),\n", xbytes(in), rf.WF, xq(rf.Diag), xq(rf.Canon), xq(rf.Dup))
	}
	o.WriteString("]\n")
	tin := cborTypedInputs(rand.New(rand.NewSource(7)), 800)
	trefs := cborTypedOracle("typed", tin)
	o.WriteString("\n/// (CBOR bytes, decoding into Person fails, the deterministic marshal of the result in hex)\nconst TYPED: List[(Bytes, Bool, Str)] = [\n")
	for i, in := range tin {
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xbytes(in), trefs[i].Err, xq(trefs[i].Out))
	}
	o.WriteString("]\n")
	tin2 := cborDeepInputs(rand.New(rand.NewSource(11)), 800)
	trefs2 := cborTypedOracle("typed2", tin2)
	o.WriteString("\n/// (CBOR bytes, decoding into Deep fails, the deterministic marshal of the result in hex)\nconst TYPED2: List[(Bytes, Bool, Str)] = [\n")
	for i, in := range tin2 {
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xbytes(in), trefs2[i].Err, xq(trefs2[i].Out))
	}
	o.WriteString("]\n")
	tin3 := cborPairInputs(rand.New(rand.NewSource(13)), 400)
	trefs3 := cborTypedOracle("typed3", tin3)
	o.WriteString("\n/// (CBOR bytes, decoding into Pair fails, the deterministic marshal of the result in hex)\nconst TYPED3: List[(Bytes, Bool, Str)] = [\n")
	for i, in := range tin3 {
		fmt.Fprintf(&o, "  (%s, %v, %s),\n", xbytes(in), trefs3[i].Err, xq(trefs3[i].Out))
	}
	o.WriteString("]\n")
	o.WriteString(cborTypedXo)
	o.WriteString(cborCheckXo)
	o.WriteString(cborAPITests)
	o.WriteString(cborStreamTests)
	cborOptsCorpus(&o)
	return o.String()
}

const cborCheckXo = `
/// "" when the case agrees with the oracle, else what differed.
fn check(data: Bytes, wf: Bool, diag: Str, canon: Str, dup: Str) -> Str {
  if cbor.well_formed(data) != wf {
    return "well_formed " + data.to_hex()
  }
  match cbor.decode(data) {
    Ok(v) => {
      if !wf || diag == "UTF8" {
        return "decode accepted " + data.to_hex()
      }
      if diag != "SKIP" && diag != "ERR" && v.diagnostic() != diag {
        return "diagnostic " + data.to_hex() + ": " + v.diagnostic() + " want " + diag
      }
      match cbor.decode(data, reject_dup_keys: true) {
        Ok(_) => {
          if dup == "yes" {
            return "dup accepted " + data.to_hex()
          }
          if canon != "ERR" && canon != "SKIP" {
            match cbor.encode(v, deterministic: true) {
              Ok(e) => {
                if e.to_hex() != canon {
                  return "encode " + data.to_hex() + ": " + e.to_hex() + " want " + canon
                }
              }
              Err(e) => return "encode failed " + data.to_hex() + ": " + e.display()
            }
          }
        }
        Err(_) => {
          if dup == "no" {
            return "dup refused " + data.to_hex()
          }
        }
      }
      // what encode writes decodes back to the same item
      match cbor.encode(v) {
        Ok(e) => {
          match cbor.decode(e) {
            Ok(w) => {
              if w.diagnostic() != v.diagnostic() {
                return "round trip " + data.to_hex()
              }
            }
            Err(x) => return "round trip decode " + data.to_hex() + ": " + x.display()
          }
        }
        Err(x) => return "round trip encode " + data.to_hex() + ": " + x.display()
      }
      ""
    }
    Err(_) => {
      if wf && diag != "ERR" && diag != "UTF8" {
        return "decode refused " + data.to_hex()
      }
      ""
    }
  }
}

test "decode, well_formed, diagnostic and encode agree with the oracle" {
  var bad: List[Str] = []
  for (data, wf, diag, canon, dup) in CASES {
    let m = check(data, wf, diag, canon, dup)
    if m != "" {
      bad.push(m)
    }
  }
  expect(bad[0..(if bad.len() < 20 { bad.len() } else { 20 })]) == []
}
`

const cborAPITests = `
fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? b"" }

test "from_int and to_int" {
  expect(cbor.from_int(0).diagnostic()) == "0"
  expect(cbor.from_int(-1).diagnostic()) == "-1"
  expect(cbor.from_int(9223372036854775807).diagnostic()) == "9223372036854775807"
  expect(cbor.from_int(-9223372036854775807 - 1).diagnostic()) == "-9223372036854775808"
  expect(cbor.encode(cbor.from_int(-9223372036854775807 - 1)).ok()?.to_hex()) == Some("3b7fffffffffffffff")
  expect(cbor.from_int(500).to_int()) == Some(500)
  expect(cbor.from_int(-500).to_int()) == Some(-500)
  expect(cbor.from_int(-9223372036854775807 - 1).to_int()) == Some(-9223372036854775807 - 1)
  match cbor.decode(hx("1bffffffffffffffff")) {
    Ok(v) => expect(v.to_int()) == None
    Err(_) => fault("decode")
  }
  match cbor.decode(hx("3b8000000000000000")) {
    Ok(v) => expect(v.to_int()) == None
    Err(_) => fault("decode")
  }
  expect(cbor.Value.Null.to_int()) == None
}

test "get finds a text key" {
  match cbor.decode(hx("a26161016162820203")) {
    Ok(v) => {
      expect(v.get("a").map(fn(x) { x.diagnostic() })) == Some("1")
      expect(v.get("b").map(fn(x) { x.diagnostic() })) == Some("[2, 3]")
      expect(v.get("c").map(fn(x) { x.diagnostic() })) == None
    }
    Err(_) => fault("decode")
  }
  expect(cbor.Value.Null.get("a").map(fn(x) { x.diagnostic() })) == None
}

test "decode_prefix leaves what follows" {
  match cbor.decode_prefix(hx("83010203f6")) {
    Ok((v, n)) => {
      expect(v.diagnostic()) == "[1, 2, 3]"
      expect(n) == 4
    }
    Err(_) => fault("decode_prefix")
  }
  match cbor.decode(hx("0000")) {
    Ok(_) => fault("trailing data accepted")
    Err(e) => expect(e.display()) == "cbor: 1 bytes of extraneous data starting at index 1"
  }
  match cbor.decode(hx("")) {
    Ok(_) => fault("empty accepted")
    Err(e) => expect(e.display()) == "cbor: unexpected EOF"
  }
}

test "max_depth bounds arrays, maps and tags" {
  expect(cbor.decode(hx("81818180"), max_depth: 4).is_err()) == false
  expect(cbor.decode(hx("8181818180"), max_depth: 4).is_err()) == true
  expect(cbor.decode(hx("a100a100a100a10000"), max_depth: 4).is_err()) == false
  expect(cbor.decode(hx("a100a100a100a100a10000"), max_depth: 4).is_err()) == true
  expect(cbor.decode(hx("c1c1c1c100"), max_depth: 3).is_err()) == false
  expect(cbor.decode(hx("c1c1c1c1c100"), max_depth: 3).is_err()) == true
  expect(cbor.well_formed(hx("81818180"), max_depth: 4)) == true
  expect(cbor.well_formed(hx("8181818180"), max_depth: 4)) == false
  match cbor.decode(hx("8181818180"), max_depth: 4) {
    Ok(_) => fault("deep value accepted")
    Err(e) => expect(e.display()) == "cbor: exceeded max nested level 4"
  }
}

test "a length past the data is refused before anything is allocated" {
  expect(cbor.decode(hx("9bffffffffffffffff")).is_err()) == true
  expect(cbor.decode(hx("bb7fffffffffffffff")).is_err()) == true
  expect(cbor.decode(hx("5bffffffffffffffff")).is_err()) == true
  expect(cbor.decode(hx("7b8000000000000000")).is_err()) == true
}

test "reject_dup_keys" {
  expect(cbor.decode(hx("a2616101616102")).is_err()) == false
  expect(cbor.decode(hx("a2616101616102"), reject_dup_keys: true).is_err()) == true
  expect(cbor.decode(hx("a2f93c0001fb3ff000000000000002"), reject_dup_keys: true).is_err()) == true
  expect(cbor.decode(hx("a2616101616202"), reject_dup_keys: true).is_err()) == false
}

test "the deterministic order is bytewise by encoded key" {
  // keys 100, 10, [-1], -1, "z", false
  let unsorted = hx("a6" + "186401" + "0a02" + "81182003" + "2004" + "617a05" + "f406")
  match cbor.decode(unsorted) {
    Ok(v) => {
      let det = cbor.encode(v, deterministic: true).ok()?.to_hex()
      expect(det) == "a6" + "0a02" + "186401" + "2004" + "617a05" + "81182003" + "f406"
      expect(cbor.encode(v).ok()?.to_hex()) == unsorted.to_hex()
    }
    Err(_) => fault("decode")
  }
}

test "encode refuses a simple value it cannot write" {
  expect(cbor.encode(cbor.Value.Simple(n: 24)).is_err()) == true
  expect(cbor.encode(cbor.Value.Simple(n: 31)).is_err()) == true
  expect(cbor.encode(cbor.Value.Simple(n: 256)).is_err()) == true
  expect(cbor.encode(cbor.Value.Simple(n: -1)).is_err()) == true
  expect(cbor.encode(cbor.Value.Simple(n: 19)).ok()?.to_hex()) == Some("f3")
  expect(cbor.encode(cbor.Value.Simple(n: 32)).ok()?.to_hex()) == Some("f820")
  expect(cbor.encode(cbor.Value.Simple(n: 255)).ok()?.to_hex()) == Some("f8ff")
}

test "encode writes the shortest float" {
  let want: List[(Float, Str)] = [
    (0.0, "f90000"), (1.0, "f93c00"), (1.5, "f93e00"), (65504.0, "f97bff"), (100000.0, "fa47c35000"),
    (1.1, "fb3ff199999999999a"), (5.960464477539063e-8, "f90001"), (3.4028234663852886e+38, "fa7f7fffff"),
    (1.0e+300, "fb7e37e43c8800759c"), (-4.0, "f9c400"), (-4.1, "fbc010666666666666"),
  ]
  for (f, h) in want {
    expect(cbor.encode(cbor.Value.Float(f: f)).ok()?.to_hex()) == Some(h)
  }
}

test "diagnostic notation" {
  let v = cbor.Value.Map(pairs: [
    (cbor.Value.TextString(s: "a\"b\\\n"), cbor.Value.Array(items: [cbor.from_int(1), cbor.Value.Null, cbor.Value.Undefined])),
    (cbor.from_int(-7), cbor.Value.Tag(num: 32, v: cbor.Value.TextString(s: "x"))),
    (cbor.Value.ByteString(b: hx("00ff")), cbor.Value.Simple(n: 100)),
    (cbor.Value.Bool(b: true), cbor.Value.Float(f: 0.1)),
    (cbor.Value.Float(f: 1.0e21), cbor.Value.Float(f: 1.5e-7)),
  ])
  expect(v.diagnostic()) == "{\"a\\\"b\\\\\\n\": [1, null, undefined], -7: 32(\"x\"), h'00ff': simple(100), true: 0.1, 1.0e+21: 1.5e-7}"
  expect(cbor.Value.Nint(n: 18446744073709551615).diagnostic()) == "-18446744073709551616"
  expect(cbor.Value.Float(f: 123456789012345680000.0).diagnostic()) == "123456789012345680000.0"
  expect(cbor.Value.Float(f: 0.000001).diagnostic()) == "0.000001"
  expect(cbor.Value.Float(f: 0.0000001).diagnostic()) == "1.0e-7"
  expect(cbor.Value.TextString(s: "\u{1}\u{7f}\u{e9}\u{1F600}").diagnostic()) == "\"\\u0001\\u007f\\u00e9\\ud83d\\ude00\""
}

test "the JSON bridge" {
  match cbor.decode(hx("a46161f56162f6" + "6163" + "83" + "fb3ff8000000000000" + "4201ff" + "c1" + "1a514b67b0" + "02" + "4103")) {
    Ok(v) => {
      match v.to_json() {
        Ok(j) => expect(j.encode()) == "{\"a\":true,\"b\":null,\"c\":[1.5,\"Af8\",1363896240],\"2\":\"Aw\"}"
        Err(e) => fault(e.display())
      }
    }
    Err(e) => fault(e.display())
  }
  expect(cbor.decode(hx("f97e00")).ok()?.to_json().is_err()) == true
  expect(cbor.decode(hx("fb7ff0000000000000")).ok()?.to_json().is_err()) == true
  expect(cbor.decode(hx("a1810101")).ok()?.to_json().is_err()) == true
  match json.parse_value("{\"k\":[1,-2,2.5,\"s\",null,true,{\"n\":{}}]}") {
    Ok(j) => {
      let c = cbor.from_json(j)
      expect(c.diagnostic()) == "{\"k\": [1, -2, 2.5, \"s\", null, true, {\"n\": {}}]}"
      expect(cbor.encode(c).ok()?.to_hex()) == "a1616b87" + "01" + "21" + "f94100" + "6173" + "f6" + "f5" + "a1616ea0"
    }
    Err(e) => fault("${e}")
  }
}
`

// cborOptsCases are the inputs of the decode_with corpus, each decoded
// under every option set of internal/stdtest/cborref opts.go (optSet).
// No case puts a refused item after another error fxamacker finds later
// (its well formed pass refuses counts, indefinite lengths, tags, NaN and
// infinity before its decode sees UTF-8 or simple values; std/cbor stops
// at the first in data order).
func cborOptsCases() []string {
	rep := func(s string, n int) string { return strings.Repeat(s, n) }
	pairs := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			if i < 24 {
				fmt.Fprintf(&b, "%02x00", i)
			} else {
				fmt.Fprintf(&b, "18%02x00", i)
			}
		}
		return b.String()
	}
	return []string{
		"90" + rep("00", 16), "91" + rep("00", 17), "9f" + rep("00", 16) + "ff", "9f" + rep("00", 17) + "ff",
		"910000", "99010000", "8191" + rep("00", 17),
		"b0" + pairs(16), "b1" + pairs(17), "bf" + pairs(16) + "ff", "bf" + pairs(17) + "ff", "b10000",
		"5f4161ff", "7f6161ff", "9fff", "bfff", "819fff", "a1009f00ff",
		"d86400", "d864d86400", "81d86400", "c11a514b67b0",
		"f97e00", "fa7fc00000", "fb7ff8000000000000", "f97c00", "f9fc00", "fa7f800000", "fbfff0000000000000", "f93c00", "fb3ff199999999999a",
		"e0", "f3", "f4", "f5", "f6", "f7", "f820", "f8ff", "f8fe",
		"8301f97e00d86400", "a1f4f5", "9ff4ff", "82" + "61ff" + "00",
	}
}

func cborOptsCorpus(o *strings.Builder) {
	cases := cborOptsCases()
	var in bytes.Buffer
	for _, c := range cases {
		for k := 0; k < 8; k++ {
			fmt.Fprintf(&in, "%d %s\n", k, c)
		}
	}
	cmd := exec.Command("go", "run", ".", "opts")
	cmd.Dir = filepath.Join("internal", "stdtest", "cborref")
	cmd.Stdin = &in
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the cbor oracle (opts) failed: %v\n", err)
		os.Exit(1)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != len(cases)*8 {
		panic("cbor opts oracle: wrong number of answers")
	}
	o.WriteString("\n/// decode_with: each input, then fxamacker's answer under opts_of(0) to opts_of(7): \"\" decodes, \"?\" another error, else the error text\nconst OPTS_CASES: List[(Str, List[Str])] = [\n")
	for i, c := range cases {
		var errs []string
		for k := 0; k < 8; k++ {
			var r struct {
				Err string `json:"err"`
			}
			if err := json.Unmarshal([]byte(lines[i*8+k]), &r); err != nil {
				panic(err)
			}
			errs = append(errs, xq(r.Err))
		}
		fmt.Fprintf(o, "  (%q, [%s]),\n", c, strings.Join(errs, ", "))
	}
	o.WriteString("]\n")
	o.WriteString(cborOptsTests)
}

const cborOptsTests = `
/// The DecodeOptions of internal/stdtest/cborref opts.go optSet(k).
fn opts_of(k: Int) -> cbor.DecodeOptions {
  var o = cbor.DecodeOptions{}
  if k == 0 || k == 7 { o.max_array_elements = 16 }
  if k == 1 || k == 7 { o.max_map_pairs = 16 }
  if k == 2 || k == 7 { o.reject_indefinite = true }
  if k == 3 || k == 7 { o.reject_tags = true }
  if k == 4 || k == 7 { o.reject_nan = true }
  if k == 5 || k == 7 { o.reject_inf = true }
  if k == 6 || k == 7 { o.rejected_simple = [0, 20, 255] }
  o
}

test "decode_with gives fxamacker's DecOptions answers (counts, indefinite lengths, tags, NaN, infinity, simple values)" {
  var bad: List[Str] = []
  for (h, want) in OPTS_CASES {
    for k in 0..8 {
      let got = match cbor.decode_with(hx(h), opts_of(k)) {
        Ok(_) => ""
        Err(e) => if want[k] == "?" { "?" } else { e.display() }
      }
      if got != want[k] {
        bad.push("${h} set ${k}: got ${got.quote()}, want ${want[k].quote()}")
      }
    }
  }
  expect(bad) == []
}

test "decode_with: default options are decode's, the prefix form, and the reserved simple values" {
  expect(cbor.decode_with(hx("9f0102ff"), cbor.DecodeOptions{})?) == cbor.decode(hx("9f0102ff"))?
  expect(cbor.decode_prefix_with(hx("0102"), cbor.DecodeOptions{reject_tags: true})?) == (cbor.Value.Uint(n: 1), 1)
  expect(cbor.decode_with(hx("00"), cbor.DecodeOptions{rejected_simple: [24]}).err().map(fn(e: cbor.CborErr) -> Str { e.display() })) == Some("cbor: cannot set analog for reserved simple value 24")
  expect(cbor.decode_with(hx("820000"), cbor.DecodeOptions{max_depth: 0}).is_err()) == true
  expect(cbor.decode_with(hx("a20000"  + "00" + "00"), cbor.DecodeOptions{reject_dup_keys: true}).is_err()) == true
}
`

const cborStreamTests = `
// TestStdAPICovered (2026-11-16): std/cbor_test.xo's ports of fxamacker's
// encode_test.go (TestMapSort, TestMarshalBigInt, TestMarshalTime),
// stream_test.go (the indefinite length Encoder) and diagnose_test.go,
// with fxamacker's expected values, as corpus calls

fn ct(s: Str) -> cbor.Value { cbor.Value.TextString(s: s) }

fn ci(n: Int) -> cbor.Value { cbor.from_int(n) }

test "TestMapSort: length first, bytewise, canonical and CTAP2 orders" {
  let m = cbor.Value.Map(pairs: [
    (ci(10), cbor.Value.Bool(b: true)),
    (ci(100), cbor.Value.Bool(b: true)),
    (ci(-1), cbor.Value.Bool(b: true)),
    (ct("z"), ct("zzz")),
    (ct("aa"), cbor.Value.Bool(b: true)),
    (cbor.Value.Array(items: [ci(100)]), cbor.Value.Bool(b: true)),
    (cbor.Value.Array(items: [ci(-1)]), cbor.Value.Bool(b: true)),
    (cbor.Value.Bool(b: false), cbor.Value.Bool(b: true)),
  ])
  // sorted keys: 10, -1, false, 100, "z", [-1], "aa", [100]
  let len_first = "a80af520f5f4f51864f5617a637a7a7a8120f5626161f5811864f5"
  // sorted keys: 10, 100, -1, "z", "aa", [100], [-1], false
  let bytewise = "a80af51864f520f5617a637a7a7a626161f5811864f58120f5f4f5"
  expect((cbor.encode_sorted(m, cbor.KeyOrder.LengthFirst) ?? e => fault(e.display())).to_hex()) == len_first
  expect((cbor.encode_sorted(m, cbor.KeyOrder.Bytewise) ?? e => fault(e.display())).to_hex()) == bytewise
  expect((cbor.encode(m, deterministic: true) ?? e => fault(e.display())).to_hex()) == bytewise
  // unsorted keeps the pairs' order
  expect((cbor.encode_sorted(cbor.Value.Map(pairs: [(ct("b"), ci(1)), (ct("a"), ci(2))]), cbor.KeyOrder.Unsorted) ?? e => fault(e.display())).to_hex()) == "a2616201616102"
}

test "TestIndefiniteByteString and TestIndefiniteTextString" {
  var e = cbor.encoder()
  e.start_indefinite(cbor.Indefinite.ByteString)
  e.encode(cbor.Value.ByteString(b: hx("0102"))) ?? x => fault(x.display())
  e.encode(cbor.Value.ByteString(b: hx("030405"))) ?? x => fault(x.display())
  e.end_indefinite() ?? x => fault(x.display())
  expect(e.bytes().to_hex()) == "5f42010243030405ff"
  var s = cbor.encoder()
  s.start_indefinite(cbor.Indefinite.TextString)
  s.encode(ct("strea")) ?? x => fault(x.display())
  s.encode(ct("ming")) ?? x => fault(x.display())
  s.end_indefinite() ?? x => fault(x.display())
  expect(s.bytes().to_hex()) == "7f657374726561646d696e67ff"
  // and it decodes back to the joined string
  expect(cbor.decode(s.bytes()) ?? x => fault(x.display())) == ct("streaming")
}

test "TestIndefiniteByteStringError and TestIndefiniteTextStringError" {
  var e = cbor.encoder()
  e.start_indefinite(cbor.Indefinite.ByteString)
  match e.encode(cbor.Value.Array(items: [ci(1), ci(2)])) {
    Ok(_) => fault("an array chunk passed")
    Err(x) => expect(x.display()) == "cbor: cannot encode item type array for indefinite-length byte string"
  }
  match e.encode(ct("hello")) {
    Ok(_) => fault("a text chunk passed")
    Err(x) => expect(x.display()) == "cbor: cannot encode item type UTF-8 text string for indefinite-length byte string"
  }
  var s = cbor.encoder()
  s.start_indefinite(cbor.Indefinite.TextString)
  match s.encode(cbor.Value.ByteString(b: hx("0102"))) {
    Ok(_) => fault("a byte chunk passed")
    Err(x) => expect(x.display()) == "cbor: cannot encode item type byte string for indefinite-length text string"
  }
}

test "TestIndefiniteArray and TestIndefiniteMap" {
  var e = cbor.encoder()
  e.start_indefinite(cbor.Indefinite.Array)
  e.encode(ci(1)) ?? x => fault(x.display())
  e.encode(cbor.Value.Array(items: [ci(2), ci(3)])) ?? x => fault(x.display())
  e.start_indefinite(cbor.Indefinite.Array)
  e.encode(ci(4)) ?? x => fault(x.display())
  e.encode(ci(5)) ?? x => fault(x.display())
  e.end_indefinite() ?? x => fault(x.display())
  e.end_indefinite() ?? x => fault(x.display())
  expect(e.bytes().to_hex()) == "9f018202039f0405ffff"
  var m = cbor.encoder(cbor.KeyOrder.LengthFirst)
  m.start_indefinite(cbor.Indefinite.Map)
  m.encode(ct("a")) ?? x => fault(x.display())
  m.encode(ci(1)) ?? x => fault(x.display())
  m.encode(ct("b")) ?? x => fault(x.display())
  m.start_indefinite(cbor.Indefinite.Array)
  m.encode(ci(2)) ?? x => fault(x.display())
  m.encode(ci(3)) ?? x => fault(x.display())
  m.end_indefinite() ?? x => fault(x.display())
  m.end_indefinite() ?? x => fault(x.display())
  expect(m.bytes().to_hex()) == "bf61610161629f0203ffff"
}

test "TestIndefiniteLengthError: a break outside an indefinite item" {
  var e = cbor.encoder()
  e.start_indefinite(cbor.Indefinite.ByteString)
  e.end_indefinite() ?? x => fault(x.display())
  match e.end_indefinite() {
    Ok(_) => fault("a second break passed")
    Err(x) => expect(x.display()) == "cbor: cannot encode \"break\" code outside indefinite length values"
  }
}

test "diagnose_test.go: byte strings in base16, base32, base32hex and base64url" {
  let v = cbor.decode(hx("4412345678")) ?? x => fault(x.display())
  expect(v.diagnostic_with(cbor.DiagBytes.Base16)) == "h'12345678'"
  expect(v.diagnostic_with(cbor.DiagBytes.Base32)) == "b32'CI2FM6A'"
  expect(v.diagnostic_with(cbor.DiagBytes.Base32Hex)) == "h32'28Q5CU0'"
  expect(v.diagnostic_with(cbor.DiagBytes.Base64)) == "b64'EjRWeA'"
  // nested, as the mode applies to every byte string
  let n = cbor.Value.Array(items: [cbor.Value.ByteString(b: b"hello"), cbor.Value.Map(pairs: [(ct("k"), cbor.Value.ByteString(b: b"\x01"))])])
  expect(n.diagnostic_with(cbor.DiagBytes.Base64)) == "[b64'aGVsbG8', {\"k\": b64'AQ'}]"
}

// encode_test.go TestMarshalBigInt and decode_test.go's bignum cases
test "from_big and to_big: shortest, always a tag, reject (BigIntConvertMode)" {
  let cases: List[(Str, Str, Str)] = [
    // (decimal, shortest hex, tag hex)
    ("0", "00", "c240"),
    ("1", "01", "c24101"),
    ("-1", "20", "c340"),
    ("18446744073709551615", "1bffffffffffffffff", "c248ffffffffffffffff"),
    ("18446744073709551616", "c249010000000000000000", "c249010000000000000000"),
    ("-18446744073709551616", "3bffffffffffffffff", "c348ffffffffffffffff"),
    ("-18446744073709551617", "c349010000000000000000", "c349010000000000000000"),
  ]
  for (dec, short, tag) in cases {
    let n = big.parse(dec) ?? e => fault(e.display())
    let a = cbor.from_big(n) ?? e => fault(e.display())
    expect((cbor.encode(a) ?? e => fault(e.display())).to_hex()) == short
    let b = cbor.from_big(n, cbor.BigConvert.None) ?? e => fault(e.display())
    expect((cbor.encode(b) ?? e => fault(e.display())).to_hex()) == tag
    expect((a.to_big() ?? fault("no big")).text()) == dec
    expect((b.to_big() ?? fault("no big")).text()) == dec
  }
  match cbor.from_big(big.from_int(1), cbor.BigConvert.Reject) {
    Ok(_) => fault("Reject encoded")
    Err(e) => expect(e.display()) == "cbor: unsupported type: big.Int"
  }
  expect(cbor.Value.TextString(s: "1").to_big() is None) == true
}

test "cbor.DecodeOptions.reject_bignum (BignumTagForbidden)" {
  let data = hx("c249010000000000000000")
  expect(cbor.decode_with(data, cbor.DecodeOptions{}) is Ok(_)) == true
  match cbor.decode_with(data, cbor.DecodeOptions{reject_bignum: true}) {
    Ok(_) => fault("a bignum passed")
    Err(e) => expect(e.display()) == "cbor: data item of cbor type tag is not accepted by protocol: bignum"
  }
  // other tags still pass
  expect(cbor.decode_with(hx("c11a514b67b0"), cbor.DecodeOptions{reject_bignum: true}) is Ok(_)) == true
}

// encode_test.go TestMarshalTime's modes and decode_test.go's time cases
test "from_time_as: TimeMode, with and without the tag (EncTagRequired)" {
  let tm = Time.from_unix_nanos(1363896240500000000)
  let w = Time.from_unix(1363896240)
  expect(cbor.from_time_as(w, cbor.TimeFormat.Unix)) == cbor.from_int(1363896240)
  expect((cbor.encode(cbor.from_time_as(w, cbor.TimeFormat.Unix, tag: true)) ?? e => fault(e.display())).to_hex()) == "c11a514b67b0"
  expect(cbor.from_time_as(tm, cbor.TimeFormat.Unix)) == cbor.from_int(1363896240)
  expect(cbor.from_time_as(tm, cbor.TimeFormat.UnixMicro)) == cbor.Value.Float(f: 1363896240.5)
  expect(cbor.from_time_as(tm, cbor.TimeFormat.UnixDynamic)) == cbor.Value.Float(f: 1363896240.5)
  expect(cbor.from_time_as(w, cbor.TimeFormat.UnixDynamic)) == cbor.from_int(1363896240)
  expect(cbor.from_time_as(tm, cbor.TimeFormat.Rfc3339)) == ct("2013-03-21T20:04:00Z")
  expect(cbor.from_time_as(tm, cbor.TimeFormat.Rfc3339Nano)) == ct("2013-03-21T20:04:00.5Z")
  // RFC 8949 Appendix A: 0("2013-03-21T20:04:00Z")
  expect((cbor.encode(cbor.from_time_as(w, cbor.TimeFormat.Rfc3339, tag: true)) ?? e => fault(e.display())).to_hex()) == "c074323031332d30332d32315432303a30343a30305a"
}

test "cbor.Value.to_time: tags under DecTagMode, byte strings, errors" {
  let want = Time.from_unix(1363896240)
  for h in ["c074323031332d30332d32315432303a30343a30305a", "c11a514b67b0", "1a514b67b0", "74323031332d30332d32315432303a30343a30305a"] {
    let v = cbor.decode(hx(h)) ?? e => fault(e.display())
    expect(v.to_time() ?? e => fault(e.display())) == want
  }
  expect(cbor.decode(hx("c1fb41d452d9ec200000")).ok().map(fn(v: cbor.Value) -> Int { (v.to_time() ?? e => fault(e.display())).unix_millis() })) == Some(1363896240500)
  // DecTagRequired refuses an untagged time; DecTagIgnored skips any tag
  match cbor.from_int(1363896240).to_time(cbor.TagRule.Required) {
    Ok(_) => fault("an untagged time passed Required")
    Err(e) => expect(e.display()) == "cbor: cannot unmarshal integer into Go value of type time.Time (expect CBOR tag value)"
  }
  let tagged2 = cbor.Value.Tag(num: UInt64.from_bits(2), v: cbor.from_int(1363896240))
  match tagged2.to_time() {
    Ok(_) => fault("tag 2 passed")
    Err(e) => expect(e.display()) == "cbor: wrong tag number for time.Time, got 2, expect 0 or 1"
  }
  expect(tagged2.to_time(cbor.TagRule.Ignored) ?? e => fault(e.display())) == want
  // ByteStringToTimeMode
  let bs = cbor.Value.ByteString(b: b"2013-03-21T20:04:00Z")
  match bs.to_time() {
    Ok(_) => fault("a byte string passed")
    Err(e) => expect(e.display()) == "cbor: cannot unmarshal byte string into Go value of type time.Time"
  }
  expect(bs.to_time(bytes: true) ?? e => fault(e.display())) == want
  expect(cbor.Value.Null.to_time() ?? e => fault(e.display())) == Time.UNIX_EPOCH
}
`
