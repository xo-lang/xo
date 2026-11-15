# GlobalISel and SelectionDAG disagree on a large aggregate argument

Status: Observed (2026-11-15, the x509 CertPool constraint check under the
LLVM test split)

Symptom: under `xo test --backend=llvm`, a closure of type
`fn(Int, lx.Cert, List[lx.Cert]) -> Str?` was called with a one-element
chain, yet it saw an empty list (`chain.len()` 0, then an index fault).
With some split counts it crashed with SIGBUS instead. The same test was
correct with `XO_LLVM_TEST_SPLIT=off`, with `XO_LLVM_OPT` set (which skips
the test split), on arm64 native, on the Go backend, and in every
`xo build --backend=llvm`, at -O0 or -O2, split or not.

Cause: Apple clang 21 on arm64 selects instructions with GlobalISel at
-O0 and with SelectionDAG at -O1 and up. The two lower a first-class
aggregate argument that does not fit in registers differently: a 59-field
`x509cert.Cert` by value (an fn-value call keeps aggregates first class;
decision 0145 passes 256 bytes and up by reference on direct calls only)
put the argument after it in another stack slot. A test build's split
compiles at -O0, except that decision 0619 compiles any module holding a
constant time module's function at -O2. The module with
`x509verify.build` (the caller) had such functions and was compiled by
SelectionDAG; the closure's module had none and was compiled by GlobalISel.
A plain -O0 build is not safe either, because GlobalISel falls back to
SelectionDAG one function at a time. A two-module IR pair (a caller at -O2
and a callee at -O0, the Cert type, one indirect call) reproduces it in
internal/llvm `TestISelMixedLevels`.

A second bug hid the first: the test binary cache key had no module count
and no xo identity, so the split counts 2, 8, 32 and `files` all fetched the
one bad binary. Only `XO_BIN_CACHE=off` showed that split 2 was correct.

Fix: every clang compile of xo's IR for arm64 at -O0 passes
`-fno-global-isel` (compiler/xo split.xo `isel_flags`: split modules,
the single module build, and a static library's IR). A C compile or a link
never gets it, since gcc has no such flag. The selector is part of the
object cache's arguments and of the binary key. Any later per-module
optimization level must keep one instruction selector across modules.
