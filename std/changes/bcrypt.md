# std/bcrypt changes

## 2026-11-03: port: std/bcrypt, golang.org/x/crypto/bcrypt

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2) and the std-coverage.md planned row (verify hashes migrated from other systems)
- Go reference: golang.org/x/crypto v0.58.0 bcrypt (bcrypt.go, base64.go) and blowfish (cipher.go, block.go, const.go)
- Test: internal/stdtest/testdata/corpus/bcrypt (hashes Go made with seeded salts at costs 4 to 6, and malformed or odd hashes against Go's CompareHashAndPassword and Cost texts) on arm64, LLVM and the Go backend; bench/perf/bcrypt twincheck
- Timing: see bench/results/std-twin-rows9-2026-11-03.md

`bcrypt.hash`, `compare` and `cost` (spec/std.md 53). One source (std/native/bcrypt.xo) for the native backends and, through std/go_compiled.txt, the Go backend.
