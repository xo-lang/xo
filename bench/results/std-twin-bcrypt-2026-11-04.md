# std/bcrypt: Blowfish state as List[UInt32] (tried 2026-11-04, not kept)

Host 1 (n=6, stopped early), bench/perf/bcrypt hash: the state (P array and S boxes) as a `List[UInt32]` with `Int(st[i])` loads and `UInt32(v)` stores, against the `List[Int]` state: wall 1.137 of before (1.12 to 1.143), worse, so not kept. The state is 4 KB in place of 8.3 KB, but each lookup gained a width conversion and the bounds check stayed.

A raw buffer (`raw.bytes_buf(4168)`, `rt.mem_load(sp, 4 * i, 4)` for each lookup) passed the arm64 and LLVM corpora but not the Go backend: std/bcrypt is in go_compiled.txt (the Go backend compiles it from the native source), where the raw and rt intrinsics do not exist. Next experiment: make the bounds checks disappear in the List[Int] version (the S box index is `(x >> 24)`, `(x >> 16) & 255` and so on, always below 256), or take bcrypt out of go_compiled.txt and give std/rt a Go implementation over golang.org/x/crypto/blowfish.
