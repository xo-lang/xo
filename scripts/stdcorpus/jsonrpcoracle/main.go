// Command jsonrpcoracle prints the std/jsonrpc corpus (decision 0603
// stage 1):
//
//   - the JSON-RPC 2.0 specification's examples (section 7,
//     https://www.jsonrpc.org/specification), each request text with the
//     answer the specification gives, through jsonrpc.answer with the
//     examples' methods (subtract, sum, get_data, update, notify_*);
//   - github.com/modelcontextprotocol/go-sdk v1.8.0's jsonrpc.DecodeMessage
//     on hand-written and seeded random texts, valid and not, rendered the
//     same way on both sides;
//   - its jsonrpc.EncodeMessage of the messages it decoded, which
//     jsonrpc.encode must write byte for byte.
//
// Where the Go SDK's encoding departs from the specification, the encode
// cases leave the input out (the decode cases keep it): it leaves a null
// response id out, where the specification (and jsonrpc.encode) writes
// "id": null; it leaves an empty method out (an omitempty string), though
// method is required; and it copies params and results through as raw
// text, so a U+2028 in a string stays unescaped where its encoder (and
// json.Value.encode) escapes it.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
)

func xq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '$':
			b.WriteString(`\$`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u{%x}`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// specExamples: the request text and the answer, "" for none. The
// specification lists a batch's answers in an order of its own; answer
// keeps the requests' order, so these are listed in that order.
var specExamples = []struct{ name, req, resp string }{
	{"positional parameters", `{"jsonrpc": "2.0", "method": "subtract", "params": [42, 23], "id": 1}`, `{"jsonrpc":"2.0","id":1,"result":19}`},
	{"positional parameters, swapped", `{"jsonrpc": "2.0", "method": "subtract", "params": [23, 42], "id": 2}`, `{"jsonrpc":"2.0","id":2,"result":-19}`},
	{"named parameters", `{"jsonrpc": "2.0", "method": "subtract", "params": {"subtrahend": 23, "minuend": 42}, "id": 3}`, `{"jsonrpc":"2.0","id":3,"result":19}`},
	{"named parameters, reordered", `{"jsonrpc": "2.0", "method": "subtract", "params": {"minuend": 42, "subtrahend": 23}, "id": 4}`, `{"jsonrpc":"2.0","id":4,"result":19}`},
	{"a notification", `{"jsonrpc": "2.0", "method": "update", "params": [1,2,3,4,5]}`, ""},
	{"a notification without params", `{"jsonrpc": "2.0", "method": "foobar"}`, ""},
	{"a non-existent method", `{"jsonrpc": "2.0", "method": "foobar", "id": "1"}`, `{"jsonrpc":"2.0","id":"1","error":{"code":-32601,"message":"Method not found"}}`},
	{"invalid JSON", `{"jsonrpc": "2.0", "method": "foobar, "params": "bar", "baz]`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}`},
	{"an invalid Request object", `{"jsonrpc": "2.0", "method": 1, "params": "bar"}`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}}`},
	{"a batch, invalid JSON", `[
  {"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
  {"jsonrpc": "2.0", "method"
]`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"Parse error"}}`},
	{"an empty array", `[]`, `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}}`},
	{"an invalid batch, but not empty", `[1]`, `[{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}}]`},
	{"an invalid batch", `[1,2,3]`, `[{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}},{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}},{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}}]`},
	{"a batch", `[
        {"jsonrpc": "2.0", "method": "sum", "params": [1,2,4], "id": "1"},
        {"jsonrpc": "2.0", "method": "notify_hello", "params": [7]},
        {"jsonrpc": "2.0", "method": "subtract", "params": [42,23], "id": "2"},
        {"foo": "boo"},
        {"jsonrpc": "2.0", "method": "foo.get", "params": {"name": "myself"}, "id": "5"},
        {"jsonrpc": "2.0", "method": "get_data", "id": "9"}
    ]`, `[{"jsonrpc":"2.0","id":"1","result":7},{"jsonrpc":"2.0","id":"2","result":19},{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"Invalid Request"}},{"jsonrpc":"2.0","id":"5","error":{"code":-32601,"message":"Method not found"}},{"jsonrpc":"2.0","id":"9","result":["hello",5]}]`},
	{"a batch of notifications", `[
        {"jsonrpc": "2.0", "method": "notify_sum", "params": [1,2,4]},
        {"jsonrpc": "2.0", "method": "notify_hello", "params": [7]}
    ]`, ""},
}

// canon is v's JSON with object keys sorted and numbers as float64, the
// form both sides render params, results and data in.
func canon(raw []byte) string {
	if len(raw) == 0 {
		return "-"
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		panic(err)
	}
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func renderID(id jsonrpc.ID) string {
	switch v := id.Raw().(type) {
	case nil:
		return "none"
	case int64:
		return fmt.Sprintf("int %d", v)
	case string:
		return "str " + v
	}
	panic("id")
}

// render is the text the Xo test's show makes of a decode.
func render(text string) string {
	m, err := jsonrpc.DecodeMessage([]byte(text))
	if err != nil {
		return "error"
	}
	switch m := m.(type) {
	case *jsonrpc.Request:
		return fmt.Sprintf("request id=%s method=%s params=%s", renderID(m.ID), m.Method, canon(m.Params))
	case *jsonrpc.Response:
		s := fmt.Sprintf("response id=%s", renderID(m.ID))
		if m.Error != nil {
			we := m.Error.(*jsonrpc.Error)
			return s + fmt.Sprintf(" error=%d/%s/%s", we.Code, we.Message, canon(we.Data))
		}
		return s + " result=" + canon(m.Result)
	}
	panic("message")
}

var decodeCases = []string{
	`{"jsonrpc":"2.0","id":1,"method":"m","params":{"a":1}}`,
	`{"jsonrpc":"2.0","method":"m"}`,
	`{"jsonrpc":"2.0","method":""}`,
	`{"jsonrpc":"2.0","id":null,"method":"m"}`,
	`{"jsonrpc":"2.0","id":"abc","method":"m","params":[1,"x",null,true]}`,
	`{"jsonrpc":"2.0","id":1.5,"method":"m"}`,
	`{"jsonrpc":"2.0","id":-7,"method":"m"}`,
	`{"jsonrpc":"2.0","id":1e3,"method":"m"}`,
	`{"jsonrpc":"2.0","id":true,"method":"m"}`,
	`{"jsonrpc":"2.0","id":{},"method":"m"}`,
	`{"jsonrpc":"2.0","method":1}`,
	`{"jsonrpc":"1.0","method":"m"}`,
	`{"method":"m","id":1}`,
	`{"jsonrpc":2,"method":"m"}`,
	`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
	`{"jsonrpc":"2.0","id":"r","result":null}`,
	`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"no such method"}}`,
	`{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"boom","data":{"why":"x"}}}`,
	`{"jsonrpc":"2.0","id":1,"error":{"code":5}}`,
	`{"jsonrpc":"2.0","id":1,"error":null,"result":3}`,
	`{"jsonrpc":"2.0","result":3}`,
	`{"jsonrpc":"2.0"}`,
	`[]`,
	`1`,
	`"x"`,
	`{"jsonrpc":"2.0","id":1,"method":"m","extra":5}`,
	`{"jsonrpc":"2.0","id":1,"method":"m","method":"n"}`,
	`{"jsonrpc":"2.0","id":2,"params":[1],"method":"later"}`,
	`{"jsonrpc":"2.0","id":3,"method":"m","params":" <&>"}`,
}

var (
	methods = []string{"initialize", "tools/call", "notifications/progress", "x", "a/b.c", "é"}
	vals    = []string{`1`, `-2`, `"s"`, `true`, `null`, `[]`, `{}`, `[1,2]`, `{"a":1,"b":[true,"x"]}`, `"<&>"`, `"tab\there"`, `{"k":{"n":"v"}}`, `0.5`}
)

// randMessage is a compact message text with sorted keys in params and
// results, and its members in a random order.
func randMessage(rg *rand.Rand) string {
	var f []string
	f = append(f, `"jsonrpc":"2.0"`)
	id := ""
	switch rg.Intn(4) {
	case 0:
	case 1:
		id = fmt.Sprintf(`%d`, rg.Intn(2000)-1000)
	default:
		id = fmt.Sprintf(`"id-%d"`, rg.Intn(100))
	}
	if rg.Intn(3) > 0 {
		f = append(f, `"method":"`+methods[rg.Intn(len(methods))]+`"`)
		if id != "" {
			f = append(f, `"id":`+id)
		}
		if rg.Intn(3) > 0 {
			f = append(f, `"params":`+vals[5+rg.Intn(len(vals)-5)])
		}
	} else {
		if id == "" {
			id = "1"
		}
		f = append(f, `"id":`+id)
		if rg.Intn(2) == 0 {
			f = append(f, `"result":`+vals[rg.Intn(len(vals))])
		} else {
			e := fmt.Sprintf(`{"code":%d,"message":"m%d"`, -32000-rg.Intn(800), rg.Intn(9))
			if rg.Intn(2) == 0 {
				e += `,"data":` + vals[rg.Intn(len(vals))]
			}
			f = append(f, `"error":`+e+`}`)
		}
	}
	rg.Shuffle(len(f), func(i, j int) { f[i], f[j] = f[j], f[i] })
	return "{" + strings.Join(f, ",") + "}"
}

func main() {
	rg := rand.New(rand.NewSource(603))
	var o strings.Builder
	o.WriteString(`// Generated by scripts/stdcorpus/jsonrpcoracle: the JSON-RPC 2.0
// specification's examples and github.com/modelcontextprotocol/go-sdk
// v1.8.0's jsonrpc package (MIT). Do not edit: regenerate with
// go run ./scripts/stdcorpus jsonrpc.

use std/json
use std/jsonrpc

fn int_at(v: json.Value?, i: Int, key: Str) -> Int {
  let p = v ?? json.Value.Null
  let x = match p {
    json.Value.List(xs) => xs.get(i)
    _ => p.get(key)
  }
  match x {
    Some(json.Value.Int(n)) => n
    _ => 0
  }
}

/// The specification's example methods.
fn handle(r: jsonrpc.Request) -> json.Value ! jsonrpc.RpcError {
  if r.method == "subtract" {
    return Ok(json.Value.Int(int_at(r.params, 0, "minuend") - int_at(r.params, 1, "subtrahend")))
  }
  if r.method == "sum" {
    var t = 0
    if let Some(json.Value.List(xs)) = r.params {
      for x in xs {
        if let json.Value.Int(n) = x {
          t += n
        }
      }
    }
    return Ok(json.Value.Int(t))
  }
  if r.method == "get_data" {
    return Ok(json.Value.List([json.Value.Str("hello"), json.Value.Int(5)]))
  }
  if r.method == "update" || r.method.starts_with("notify_") {
    return Ok(json.Value.Null)
  }
  Err(jsonrpc.error_of(jsonrpc.METHOD_NOT_FOUND))
}

/// v as the oracle renders JSON: keys sorted, numbers as Go's float64.
fn canon(v: json.Value?) -> Str {
  match v {
    None => "-"
    Some(x) => canon_value(x)
  }
}

fn canon_value(v: json.Value) -> Str {
  match v {
    json.Value.Object(fs) => {
      var ks: List[Str] = []
      for (k, _) in fs {
        ks.push(k)
      }
      ks = ks.sort()
      var parts: List[Str] = []
      for k in ks {
        parts.push(json.Value.Str(k).encode() + ":" + canon_value(v.get(k) ?? json.Value.Null))
      }
      "{" + parts.join(",") + "}"
    }
    json.Value.List(xs) => {
      var parts: List[Str] = []
      for x in xs {
        parts.push(canon_value(x))
      }
      "[" + parts.join(",") + "]"
    }
    _ => v.encode()
  }
}

fn show_id(id: jsonrpc.Id?) -> Str {
  match id {
    None => "none"
    Some(jsonrpc.Id.Int(n)) => "int ${n}"
    Some(jsonrpc.Id.Str(s)) => "str ${s}"
  }
}

fn show(text: Str) -> Str {
  match jsonrpc.decode(text) {
    Err(_) => "error"
    Ok(jsonrpc.Message.Req(r)) => "request id=${show_id(r.id)} method=${r.method} params=${canon(r.params)}"
    Ok(jsonrpc.Message.Resp(r)) => {
      let s = "response id=${show_id(r.id)}"
      match r.error {
        Some(e) => s + " error=${e.code}/${e.message}/${canon(e.data)}"
        None => s + " result=${canon(Some(r.result))}"
      }
    }
  }
}

fn reencode(text: Str) -> Str {
  match jsonrpc.decode(text) {
    Ok(m) => jsonrpc.encode(m)
    Err(e) => "error ${e.code}"
  }
}
`)
	for _, c := range specExamples {
		want := "None"
		if c.resp != "" {
			want = "Some(" + xq(c.resp) + ")"
		}
		fmt.Fprintf(&o, "\ntest %s {\n  expect(jsonrpc.answer(%s, handle)) == %s\n}\n", xq("specification example: "+c.name), xq(c.req), want)
	}
	for i, c := range decodeCases {
		fmt.Fprintf(&o, "\ntest %s {\n  expect(show(%s)) == %s\n}\n", xq(fmt.Sprintf("decode as the Go SDK, case %d", i)), xq(c), xq(render(c)))
	}
	var rs []string
	for i := 0; i < 80; i++ {
		rs = append(rs, randMessage(rg))
	}
	sort.Strings(rs)
	for i, t := range rs {
		m, err := jsonrpc.DecodeMessage([]byte(t))
		if err != nil {
			panic(t + ": " + err.Error())
		}
		enc, err := jsonrpc.EncodeMessage(m)
		if err != nil {
			panic(err)
		}
		fmt.Fprintf(&o, "\ntest %s {\n  let t = %s\n  expect(show(t)) == %s\n  expect(reencode(t)) == %s\n}\n", xq(fmt.Sprintf("random message %d: decode and encode as the Go SDK", i)), xq(t), xq(render(t)), xq(string(enc)))
	}
	o.WriteString(fixed)
	fmt.Print(o.String())
}

// fixed is the framing tests: the formats' rules, not an oracle's output.
const fixed = `
fn lines(chunks: List[Bytes], max: Int = 64) -> Str {
  var r = jsonrpc.line_reader(max: max)
  var out: List[Str] = []
  for c in chunks {
    match r.feed(c) {
      Ok(ls) => {
        for l in ls {
          out.push(l)
        }
      }
      Err(e) => {
        out.push("error: ${e.display()}")
        break
      }
    }
  }
  out.join("|")
}

fn bodies(chunks: List[Bytes], max: Int = 64) -> Str {
  var r = jsonrpc.header_reader(max: max)
  var out: List[Str] = []
  for c in chunks {
    match r.feed(c) {
      Ok(bs) => {
        for b in bs {
          out.push(b)
        }
      }
      Err(e) => {
        out.push("error: ${e.display()}")
        break
      }
    }
  }
  out.join("|")
}

test "the newline framing: lines across chunks, CRLF, empty lines skipped" {
  expect(lines([b"{\"a\":1}\n{\"b\"", b":2}\r\n\n", b"{\"c\":3}"])) == "{\"a\":1}|{\"b\":2}"
  expect(lines([jsonrpc.line_frame("{}"), jsonrpc.line_frame("[]")])) == "{}|[]"
}

test "the newline framing: a line over the limit" {
  expect(lines([b"0123456789\n"], max: 4)) == "error: jsonrpc: message longer than 4 bytes"
  expect(lines([b"01234", b"56789"], max: 4)) == "error: jsonrpc: message longer than 4 bytes"
}

test "the Content-Length framing: bodies across chunks, headers without case, other headers ignored" {
  let two = jsonrpc.header_frame("{\"a\":1}") + b"content-length: 2\r\nContent-Type: application/vscode-jsonrpc; charset=utf-8\r\n\r\n{}"
  expect(bodies([two])) == "{\"a\":1}|{}"
  var chunks: List[Bytes] = []
  for i in 0..two.len() {
    chunks.push(two[i..i + 1])
  }
  expect(bodies(chunks)) == "{\"a\":1}|{}"
  expect(jsonrpc.header_frame("\u{e9}")) == b"Content-Length: 2\r\n\r\n\xc3\xa9"
}

test "the Content-Length framing: missing and bad lengths, a body over the limit" {
  expect(bodies([b"Content-Type: x\r\n\r\n{}"])) == "error: jsonrpc: no Content-Length header"
  expect(bodies([b"Content-Length: x\r\n\r\n"])) == "error: jsonrpc: invalid Content-Length \"x\""
  expect(bodies([b"Content-Length: -1\r\n\r\n"])) == "error: jsonrpc: invalid Content-Length \"-1\""
  expect(bodies([b"Content-Length: 99\r\n\r\n"], max: 64)) == "error: jsonrpc: message longer than 64 bytes"
  expect(bodies([b"garbage\r\n\r\n"])) == "error: jsonrpc: malformed header line \"garbage\""
}

test "encode writes a notification without an id and an error without a result" {
  expect(jsonrpc.encode(jsonrpc.Message.Req(jsonrpc.Request{method: "ping"}))) == "{\"jsonrpc\":\"2.0\",\"method\":\"ping\"}"
  let e = jsonrpc.RpcError{code: -32000, message: "boom", data: Some(json.Value.Str("x"))}
  expect(jsonrpc.encode(jsonrpc.Message.Resp(jsonrpc.Response{id: Some(jsonrpc.Id.Int(3)), error: Some(e)}))) == "{\"jsonrpc\":\"2.0\",\"id\":3,\"error\":{\"code\":-32000,\"message\":\"boom\",\"data\":\"x\"}}"
  expect(jsonrpc.encode(jsonrpc.Message.Resp(jsonrpc.Response{id: Some(jsonrpc.Id.Str("a"))}))) == "{\"jsonrpc\":\"2.0\",\"id\":\"a\",\"result\":null}"
}
fn pipe_conn(out: Chan[Str]) -> jsonrpc.Conn[()] {
  jsonrpc.conn(fn(t: Str) -> () ! IoErr {
    out.send(t)
    Ok(())
  })
}

fn reply(x: Result[json.Value, jsonrpc.CallErr]) -> Str {
  match x {
    Ok(v) => "ok ${v.encode()}"
    Err(e) => "err ${e.display()}"
  }
}

test "Conn: calls both ways, at the same time, with the peer's errors" {
  let ab = Chan[Str].new(cap: 16)
  let ba = Chan[Str].new(cap: 16)
  let got = scope s {
    let a = pipe_conn(ab)
    let b = pipe_conn(ba)
    a.serve(s.group(), fn() -> Str? ! IoErr { Ok(ba.recv()) }, handle)
    b.serve(s.group(), fn() -> Str? ! IoErr { Ok(ab.recv()) }, handle)
    let t1 = s.spawn(fn() -> json.Value ! jsonrpc.CallErr { a.call("subtract", Some(json.Value.List([json.Value.Int(42), json.Value.Int(23)]))) })
    let t2 = s.spawn(fn() -> json.Value ! jsonrpc.CallErr { b.call("get_data") })
    let t3 = s.spawn(fn() -> json.Value ! jsonrpc.CallErr { a.call("nope") })
    let n = a.notify("notify_hello")
    let out = [reply(t1.await), reply(t2.await), reply(t3.await), "${n.is_ok()}", "${a.in_flight()}"]
    ab.close()
    ba.close()
    out
  }
  expect(got) == ["ok 19", "ok [\"hello\",5]", "err Method not found", "true", "0"]
}

test "Conn: the end of the read side ends the calls waiting, and later ones" {
  let ab = Chan[Str].new(cap: 16)
  let ba = Chan[Str].new(cap: 16)
  let got = scope s {
    let a = pipe_conn(ab)
    a.serve(s.group(), fn() -> Str? ! IoErr { Ok(ba.recv()) }, handle)
    let t = s.spawn(fn() -> json.Value ! jsonrpc.CallErr { a.call("subtract") })
    let sent = ab.recv() ?? ""
    ba.close()
    [sent, reply(t.await), reply(a.call("sum"))]
  }
  expect(got) == ["{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"subtract\"}", "err jsonrpc: connection closed", "err jsonrpc: connection closed"]
}

test "Conn: a batch and invalid text from the peer are answered" {
  let ab = Chan[Str].new(cap: 16)
  let ba = Chan[Str].new(cap: 16)
  let got = scope s {
    let a = pipe_conn(ab)
    a.serve(s.group(), fn() -> Str? ! IoErr { Ok(ba.recv()) }, handle)
    ba.send("[{\"jsonrpc\":\"2.0\",\"method\":\"sum\",\"params\":[1,2],\"id\":7},{\"jsonrpc\":\"2.0\",\"method\":\"notify_x\"}]")
    ba.send("{oops")
    let out = [ab.recv() ?? "", ab.recv() ?? ""]
    ba.close()
    out
  }
  expect(got) == ["[{\"jsonrpc\":\"2.0\",\"id\":7,\"result\":3}]", "{\"jsonrpc\":\"2.0\",\"id\":null,\"error\":{\"code\":-32700,\"message\":\"Parse error\"}}"]
}
`
