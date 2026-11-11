# std/term changes

## 2026-11-11: refactor: std/term is one file (decisions 0588, 0597)

- Kind: refactor
- Commit: this push
- Found by: decision 0597 (std/native/term.xo imports nothing native)
- Go reference: none (no behavior change)
- Test: goldens term_stdio (arm64, llvm, Go parity) and test_term (Go backend)
- Shipped in: not yet released

std/native/term.xo moved into std/term.xo. The Go backend compiles it
from that source too: the std/term part of its Go twin (std/rt/term.go)
is gone.
