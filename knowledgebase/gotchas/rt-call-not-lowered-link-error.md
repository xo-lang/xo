# A direct rt.* call that no one lowers is a link error, not a checker error

Symptom: Observed 2026-10-24, adding `Str.map_runes` to the native prelude:
the program built and then died at start-up with

```
dyld: Symbol not found: _xo_rs_push_rune
  Referenced from: <the built binary>
  Expected in:     /usr/lib/libSystem.B.dylib
```

Cause: `rt.rs_push_rune` is not a runtime function on the native backends.
It is an intrinsic the compiler lowers, and only in one place:
`compiler/ssa/call.xo` turns `StrBuilder.write_rune` into
`b.llrt_or_rt("rs_push_rune", ...)`, which names the llrt function
`llrt_rs_push_rune` (`std/native/prelude.xo`) and so `raw.push_rune`
(`std/native/llrt/raw.xo`). A call written by hand anywhere else keeps the
rt name, and the linker resolves it against the C runtime, which has no
such export, so the build succeeds and the binary fails when it starts
(`internal/repotest/testdata/intrinsic_gaps.txt` lists rs_push_rune for
arm64 and llvm as "lowered by compiler/ssa itself", which is this rule).
wasm is the exception: `compiler/wasm/rt.xo` does define `$rs_push_rune`.

Fix: call the llrt form (`raw.push_rune(s, n)`), which appends the UTF-8
of a Rune and substitutes U+FFFD for a value that is not a scalar, the way
`BytesBuilder.write_rune` does. `rt.rs_with_cap`, `rt.rs_append_bytes`,
`rt.rs_push_byte` and `rt.rs_set_len` are lowered everywhere and are safe
to call directly.

The general lesson: a build that passes is not an arm64 run; a new rt.*
call in std/native needs one program that exercises it on the native
backend before the change is committed. The compiler would do better to
refuse an rt.* name that no path lowers, with a diagnostic instead of a
link error.
