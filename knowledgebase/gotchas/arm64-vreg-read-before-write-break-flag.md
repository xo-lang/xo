# arm64: "reads virtual register N before it is written" after a flag loop with break

Status: Observed (2026-10-13, decision 0193 step 6, std/native/llrt/http1.xo)

Symptom: `xo build --backend=arm64` (the Go driver, `XO_SELF_DRIVER=0`)
fails with `arm64 backend: internal: unescape reads virtual register 20
before it is written`. The LLVM backend builds the same program and it
runs correctly.

Repro (user code, no runtime module needed):

```
fn cls(c: Int) -> Int { c & 3 }

fn unescape(b: Bytes, s: Str, lo: Int, hi: Int, mode: Int) -> (Bool, Str) {
  var i = lo
  var plain = true
  while i < hi {
    let c = Int(b[i])
    if c == 37 || (c == 43 && mode == 1) || (mode == 2 && c < 128 && (cls(c) & 2) == 0) {
      plain = false
      break
    }
    i += 1
  }
  if plain {
    return (true, s)
  }
  var out = ""
  while i < hi {
    let c = Int(b[i])
    if c == 37 {
      if i + 2 >= hi {
        return (false, "")
      }
      out = out + "%"
      i += 3
      continue
    }
    out = out + "x"
    i += 1
  }
  (true, out)
}

fn main(os: Os) {
  let s = "a%41b+c"
  let (ok, t) = unescape(s.bytes(), s, 0, s.len(), 1)
  os.stdio.println("${ok} ${t}")
}
```

A smaller version (the condition only `b[i] == 37`) builds, so the
`||` of `&&` chains in the loop's break condition is part of it; the
cause in the arm64 emitter (internal/arm64, likely mirrored in
selfhost/arm64) is not found yet.

Fix (workaround): http1.xo moves the scan into a function of its own
(`plain_end`, which returns the index instead of setting a flag and
breaking). Next: run the repro through xo1's arm64 backend, then bisect
the emitter's passes (`XO_ARM64_DEBUG`-style dumps of the virtual
register code before register allocation) to find the block whose use
has no definition on some path.
