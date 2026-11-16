# std/term changes

## 2026-11-16: perf: painting with one allocation, color_enabled at Go's speed

- Kind: perf
- Commit: this push
- Found by: the bench/perf/term Go twin (#88, ADR 0655 rule 7): tty 2.74 and paint 1.82 times golang.org/x/term v0.48.0 and the Go wrap on the Mac
- Go reference: golang.org/x/term IsTerminal (TIOCGETA), os.LookupEnv
- Test: goldens term_stdio and env_index (arm64, llvm); bench/perf/term twincheck (4 workloads agree)
- Shipped in: not yet released
- Timing: Mac, twincheck -time 7: tty 2.74 to 1.06 to 1.08 (three runs), paint 1.82 to 0.92 to 1.05, strip 0.82 to 0.90, none 0.86 to 0.91

A Painter's color wraps its text by interpolation, one allocation, not
four concatenations. color_enabled checks FORCE_COLOR and CLICOLOR_FORCE
without a list literal (one allocation a call). Its four environment
reads go through the environment's new index (prelude.md, same day) and its terminal
check on macOS is TIOCGETA through ioctl as x/term's, not the C
library's isatty (1.7 times as long per call).

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
