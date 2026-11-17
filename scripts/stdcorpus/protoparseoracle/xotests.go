package main

// xoTests renders a parsed file as outline does and checks every case.
const xoTests = `
fn field_line(pad: Str, f: protoparse.Field) -> Str {
  var label = ""
  var t = f.typ
  if f.key != "" {
    label = "map"
    t = "${f.key},${f.typ}"
  } else if f.repeated {
    label = "repeated"
  } else if f.required {
    label = "required"
  }
  "${pad} field ${f.name} ${f.num} ${label} ${t} ${f.oneof}\n"
}

fn enum_lines(pad: Str, e: protoparse.Enum) -> Str {
  "${pad}enum ${e.name} ${e.values.map(fn(v) { "${v.name}=${v.num}" }).join(",")}\n"
}

fn message_lines(pad: Str, m: protoparse.Message) -> Str {
  var s = "${pad}message ${m.name}\n"
  for f in m.fields {
    s += field_line(pad, f)
  }
  for n in m.nested {
    s += message_lines(pad + " ", n)
  }
  for e in m.enums {
    s += enum_lines(pad + " ", e)
  }
  s
}

fn outline(f: protoparse.File) -> Str {
  var s = "syntax ${f.syntax} package ${f.package} imports ${f.imports.join(",")}\n"
  for m in f.messages {
    s += message_lines("", m)
  }
  for e in f.enums {
    s += enum_lines("", e)
  }
  for v in f.services {
    s += "service ${v.name}\n"
    for r in v.rpcs {
      s += " rpc ${r.name} ${r.req} ${r.resp} ${r.client_stream} ${r.server_stream}\n"
    }
  }
  s
}

test "std/protoparse accepts what protocompile accepts, with the same outline" {
  var bad: List[Str] = []
  var n = 0
  for (src, ok, want) in CASES {
    if !ok { continue }
    n += 1
    match protoparse.parse(src) {
      Ok(f) => {
        let got = outline(f)
        if got != want {
          bad.push("outline differs:\n${src}\ngot:\n${got}want:\n${want}")
        }
      }
      Err(e) => bad.push("rejected (${e.display()}):\n${src}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${n} accepted files differ, first: ${bad[0]}")
  }
}
`

// rejectChecked turns on xoReject: on since task #230 (std/protoparse
// accepted four kinds of file protoc rejects: allow_alias with no aliases,
// overlapping reserved ranges, overlapping extension ranges, an option
// with no value).
const rejectChecked = true

// xoReject checks that every file protocompile rejects is rejected.
const xoReject = `
test "std/protoparse rejects what protocompile rejects" {
  var bad: List[Str] = []
  var n = 0
  for (src, ok, _) in CASES {
    if ok { continue }
    n += 1
    if let Ok(f) = protoparse.parse(src) {
      bad.push("accepted:\n${src}\noutline:\n${outline(f)}")
    }
  }
  if !bad.is_empty() {
    fault("${bad.len()} of ${n} rejected files differ, first: ${bad[0]}")
  }
}
`
