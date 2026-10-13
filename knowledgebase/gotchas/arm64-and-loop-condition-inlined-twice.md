# arm64: a function inlined twice under a `&&` loop condition reads a register before writing it

Status: Observed (2026-10-13, std/native/llrt/glob.xo, decision 0195);
not fixed, worked around in the source.

Symptom: `xo build --backend=arm64` (the Go driver's internal/arm64)
stops with `arm64 backend: internal: main reads virtual register 53
before it is written` (or the name of the function the copies were
inlined into). LLVM builds and runs the same program.

Cause: not found yet. The trigger, minimized from glob.xo's `matches`
(41 lines): a small function with a loop and early returns (`mc` below)
called twice from one function, once inside a `while` whose condition is
`k < s.len() && s[k] != 47`, so that both copies are inlined (decision
0184 item 1: at most 2 call sites). Without the `&& s[k] != 47` it
builds; with one call site it builds. Suspects: the `&&` join's facts
(`joinSource`, decision 0184 item 3) or `threadConst` / `threadCset`
copying moves through the join of an inlined return.

```xo
fn mc(p: Bytes, lo: Int, hi: Int, s: Bytes, si0: Int) -> Int {
  var i = lo
  var si = si0
  let n = s.len()
  while i < hi {
    if si >= n {
      return -1
    }
    if p[i] != s[si] {
      return -1
    }
    si += 1
    i += 1
  }
  si
}

fn matches(pattern: Str, name: Str) -> Bool {
  let p = pattern.bytes()
  let s = name.bytes()
  var pi = 0
  var si = 0
  while pi < p.len() {
    let lo = pi
    let hi = pi + 1
    pi = hi
    let t = mc(p, lo, hi, s, si)
    if t >= 0 {
      si = t
      continue
    }
    var next = false
    var k = si
    while k < s.len() && s[k] != 47 {
      let t2 = mc(p, lo, hi, s, k + 1)
      if t2 >= 0 {
        si = t2
        next = true
        break
      }
      k += 1
    }
    if !next {
      return false
    }
  }
  si == s.len()
}

fn main(os: Os) {
  let args = os.env.args()
  os.stdio.println("${matches(args.get(0) ?? "a*b", args.get(1) ?? "axb")}")
}
```

Fix (workaround): test the second operand as a `break` in the body
(`while k < s.len() { if s[k] == 47 { break } ... }`), as glob.xo does.
Next: build the program above with `XO_SELF_DRIVER=0 xo build
--backend=arm64` and dump the virtual code before register allocation
(internal/arm64 opt.go `liveDCE`, `threadConst`, `joinSource`), find the
pass that drops the definition, fix it in selfhost/arm64 first and mirror
it in internal/arm64 (the emitters are byte identical, decision 0158),
and add the program as a golden (internal/llvm/testdata/golden).
