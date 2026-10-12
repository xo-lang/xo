# Runtime flags turned on by a symbol name in a string literal

Status: Observed (2026-10-12, scripts/release.sh run 2 of the switch
point B flip; bench/results/retire-go-2026-10-12.md "Switch point B: the
flip").

## Symptom

`xo1 build --backend=arm64 --target=linux/arm64 --ld=macho selfhost/xo`
stopped after 899 s with `--ld=macho needs a prebuilt linux/arm64
runtime pack for the flags -O2 -DXO_FS -DXO_STDIN -DXO_PROC
-DXO_UNICODE_C`, with a complete pack in `$XO_RTPACK`. The darwin build
of the same program (cc link) succeeded, silently compiling the runtime
locally instead of using the pack.

## Cause

`build.IOFlags` and xo1's copy `io_flags` (selfhost/xo link.xo) chose
the runtime's `-D` flags by `bytes.Contains(code, sym)` over the
program's assembly or IR. xo1's own source holds those symbol names as
string literals (the tables of io_flags itself), which land in its
assembly as `.ascii "xo_str_lower"`, so building xo1 turned on
`-DXO_UNICODE_C` (decision 0180's C path), which no pack variant has by
design. With cc the runtime is then compiled locally (slower, no harm);
with `--ld=macho` there is no compiler, so the link fails.

## Fix

`callsSym` / `calls_sym` (commit 440c630, both drivers, same
rule): a symbol counts only when preceded by what a reference has before
it (space, tab, `_`, `@`, `:`, `,`), not by a string literal's quote.
TestIOFlagsUnicodeC covers `.ascii "xo_str_lower"` and `c"xo_str_lower"`.
When a flag is chosen by scanning generated code, test it on a program
that names the symbol in a string, and on selfhost/xo.
