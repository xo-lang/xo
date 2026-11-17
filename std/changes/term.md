# std/term changes

## 2026-11-17: perf: strip copies the text between escapes in runs

- Kind: perf
- Commit: this push
- Found by: #88, the bench/perf/term strip row (1.123 of Go on host 1)
- Go reference: the twin's strip (a builder of len(s), as Go programs write it)
- Test: goldens test_term (new cases: escapes at either end, an unterminated CSI, multibyte runs, an escape that cuts a UTF-8 sequence); bench/perf/term strip output matches its Go twin
- Shipped in: not yet released

strip wrote each byte with write_byte into a growing builder; it now
reserves the input's size and writes each run between escapes with one
copy. Mac, 20M painted lines: 1.41 to 1.08 s (Go 1.45 s). Host 1 `xo bench --ab` bench/perf/term strip: 0.837 of before (wall 15.02 to 12.57 ms).

## 2026-11-17: perf: strip's plain path saves no registers (#103)

- Kind: perf
- Commit: this push
- Found by: #103 lever 5 (callgrind: strip's prologue saved the registers its escape loop uses before the ESC check)
- Go reference: strings.Contains(s, "\x1b") (IndexByte)
- Test: internal/arm64 TestEscCheckFast (instruction count of 10,000,000 checks); bench/perf/strcontains strip
- Shipped in: not yet released

strip checks for ESC and returns, and only a string holding one goes to
strip_esc, so the common plain call does not set up the escape loop's
frame. 10,000,000 checks: 1,190M to under 900M instructions.

## 2026-11-16: perf: strip checks for ESC with one byte search (#103)

- Kind: perf
- Commit: this push
- Found by: #103 (term.strip's plain path, Str.contains of one byte through the general substring search)
- Go reference: strings.Contains(s, "\x1b") (IndexByte)
- Test: bench/perf/strcontains strip (output matches its Go twin); goldens term_stdio
- Shipped in: not yet released

strip's check for an escape uses contains_rune, one find_byte, instead of
Str.contains' general search. Host 1, `xo bench --ab` on
bench/perf/strcontains strip (20M calls on short lines): 0.853 of before
(100.6 to 85.8 ms). Mac: xo 0.08 s, Go 0.06 to 0.07 s.
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
## 2026-11-16: feature: the line editor, golang.org/x/term's Terminal (decision 0684)

- Kind: feature
- Commit: this push
- Found by: task #89 (map.txt's 11 term gaps)
- Go reference: golang.org/x/term v0.48.0 terminal.go (Terminal, NewTerminal, ReadLine, ReadPassword, Write, SetPrompt, SetSize, SetBracketedPasteMode, History, EscapeCodes, ErrPasteIndicator)
- Test: corpus term (arm64, llvm, go): 710 sessions through Go's Terminal over terminal_test.go's MockTerminal (keyPressTests, renderTests, TestCRLF, the password and SetSize tests at read sizes 1, 2, 3, 5 and whole, 400 seeded random sessions with widths, SetSize, Write, ReadPassword, pastes and an autocomplete callback), each line and every byte written equal to Go's; conn_port over a loopback connection, ring_history, set_prompt, the escape codes
- Timing: bench/perf/termline against golang.org/x/term v0.48.0 on linux/arm64 host 1 (xo bench --ab, 15 rounds, Xo/Go): type 0.996 (0.987 to 1.007; 1.51 before writing lines from an index and escape sequences into the output directly), wrap 1.046 (1.021 to 1.05), none 0.485; max RSS 0.40
- Shipped in: not yet released

`term.new_terminal(port, prompt)` over a `Port` (`stdio_port`, `conn_port`, `func_port`): read_line with editing keys, history and bracketed paste (`Line.pasted` is Go's ErrPasteIndicator), read_password, write above the prompt from another task, set_size, set_prompt, set_history, set_autocomplete, and the VT100 escape codes. Differences from Go: a Port read cannot return data and the end of input together (Go's dataAndEOFReader case reads once more); History and the autocomplete function run with the terminal locked (Go unlocks for them), so they must not call it.

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
