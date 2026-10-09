# LLVM: a wrapping hash emitted with `nsw` lets -O2 fold the hash away

Status: Observed (2026-10-09, while writing the `native_values_hof`
golden)

Symptom: on the LLVM backend only, `let _ = users.remove(4)` followed by
`users.remove(5)` on a `Map[Int, User]` removed nothing: `len()` stayed
7, the second remove returned None, iteration still showed both keys.
The arm64 build and the same IR at -O1 or under AddressSanitizer (-O1)
were right; `let r = users.remove(4)` (the Option kept alive) happened
to be right too. It existed before the change that found it (the xo
binary of 8762762 behaves the same).

Cause: the SSA builder documents arithmetic without `Pos` as unchecked
("wrapping"), and `xo.hash[Int]` multiplies the key by a large odd
constant on purpose, but the LLVM backend emitted every unchecked
add/sub/mul as `nsw` (overflow is poison). After inlining, LLVM knew the
key was 4 or 5 (`range(i64 4, 6)`), so `key * C` always overflowed, the
value was poison, and CorrelatedValuePropagation rewrote
`x ^ (x >> 29)` to `x ^ x` = 0: the lookup probed the wrong slot and
missed. Found with `clang -O2 -mllvm -opt-bisect-limit=N` on the build
directory's `prog.ll` (first bad pass: CorrelatedValuePropagation on
`xo.map.remove`).

Fix: arithmetic that wraps by design carries `Aux: ssa.Wrap`
(`xo.hash[Int]` and the `h * 31 + x` mix of composite hashes), and the
LLVM backend emits it without `nsw` (commit 0dc4d7f; golden
`native_map_hash_wrap`). Any new unchecked arithmetic that can overflow
(hashes, PRNGs, checksums) must be marked `Wrap`; arm64 wraps anyway, so
only an LLVM build at -O2 shows the bug. To bisect a miscompile: build
with `XO_BUILD_DIR=<dir>`, then compile `<dir>/<hash>/llvm/prog.ll` with
the runtime at -O2 and `-mllvm -opt-bisect-limit=N` (link with
`-isysroot` of the Xcode SDK, gotchas/macos-clt-sdk-link-failure.md).
