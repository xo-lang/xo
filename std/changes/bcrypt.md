# std/bcrypt changes

## 2026-11-10: refactor: one file with its bodies (decision 0588 stage 2)

- Kind: refactor
- Commit: this commit
- Found by: decision 0588 stage 2 (pure modules merged with their implementation)
- Go reference: none (no behavior change)
- Test: internal/stdtest corpus bcrypt on go, arm64 and llvm

std/bcrypt.xo now holds the bodies of std/native/bcrypt.xo (removed), and the Go
backend compiles them from it as its own package; std/rt/bcrypt.go (the Go backend's display methods) is removed, the
module's own `display` serves it. The API, texts and errors
are unchanged: every item the declaration file had is `pub`, with the same
parameter names.

## 2026-11-03: port: std/bcrypt, golang.org/x/crypto/bcrypt

- Kind: port
- Commit: this commit
- Found by: the lead's roadmap (priority 2) and the std-coverage.md planned row (verify hashes migrated from other systems)
- Go reference: golang.org/x/crypto v0.58.0 bcrypt (bcrypt.go, base64.go) and blowfish (cipher.go, block.go, const.go)
- Test: internal/stdtest/testdata/corpus/bcrypt (hashes Go made with seeded salts at costs 4 to 6, and malformed or odd hashes against Go's CompareHashAndPassword and Cost texts) on arm64, LLVM and the Go backend; bench/perf/bcrypt twincheck
- Timing: bench/results/std-twin-rows9-2026-11-03.md: hash 1.44, compare 1.55 of Go's wall; after the ubfx and load offset codegen (7bcf94107), bench/results/std-twin-rows14-2026-11-03.md: hash 1.19, compare 1.20; not Proven

`bcrypt.hash`, `compare` and `cost` (spec/std.md 53). One source (std/native/bcrypt.xo) for the native backends and, through std/go_compiled.txt, the Go backend.

## 2026-11-04: perf: blowfish's F without index or overflow checks, bcrypt at Go's speed

- Kind: perf
- Commit: this commit
- Found by: the lead's queue (Lang 3's bcrypt at 1.19 to 1.20 of Go, blocked on masked index checks)
- Go reference: Go's blowfish indexes `[256]uint32` arrays with a byte, so its compiler needs no check
- Test: test/codegen/maskedindex (`xo dev codegen`), the bcrypt corpus
- Timing: bench/results/bcrypt-sbox-2026-11-04.md (host 1): hash 0.998 and compare 0.999 of Go's wall (was 1.19 and 1.20), max RSS 0.55

expand checks the state's length (1042 words) once and f masks the top byte of its 32-bit word, so the compiler proves every S box index in range (compiler/arm64 Proof FACT_MIN, a guard on a list's length against a constant): expand's 18 index checks are 4 (the key and salt words and the stores), 0.98 of the wall. f's two adds are `wrapping_add` (every state word is below 2**32, so neither sum can overflow): 0.846 more.
