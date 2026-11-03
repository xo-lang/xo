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
	o.WriteString("use std/cbor\nuse std/json\n\n")
	o.WriteString("/// (CBOR bytes, well formed, diagnostic notation or ERR, UTF8 or SKIP, deterministic encoding in hex or ERR or SKIP, repeated map key: yes, no or ?)\nconst CASES: List[(Bytes, Bool, Str, Str, Str)] = [\n")
	for i, in := range inputs {
		rf := refs[i]
		fmt.Fprintf(&o, "  (%s, %v, %s, %s, %s),\n", xbytes(in), rf.WF, xq(rf.Diag), xq(rf.Canon), xq(rf.Dup))
	}
	o.WriteString("]\n")
	o.WriteString(cborCheckXo)
	o.WriteString(cborAPITests)
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
