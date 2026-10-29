# GNU as on x86 Linux rejected the arm64 .arch_extension directives in every cc build

Status: Observed (2026-10-28, xo-lang-6, the adopted amd64 CI runner).

## Symptom

On the amd64 CI runner (Amazon Linux 2023, cc is gcc 11.5), xotool's
seed smoke build failed with only `xo: /usr/bin/cc failed`, so no xo was
built and every job on that runner died in its first minute. The seed xo
built a hello program by hand on the same machine.

## Cause

Two things. The native backend's linux assembly prologue
(compiler/arm64/gen.xo file_header) emitted `.arch_extension crypto`,
`aes` and `sha2` for std's arm64 crypto kernels on every linux target,
x86 included; GNU as on x86 has no such directive ("unknown pseudo-op").
clang's assembler takes it on x86 too, and the default linux link is
Xo's own (no assembler at all), so the Debian Docker goldens, the Macs
and the arm64 runners never saw it: only a build through cc with gcc on
x86 did, which is what `XO_NOLIBC=0` means, and xotool sets that for
the seed smoke build and the self build on linux. Then xotool's
firstErrors kept only lines with `: error ` (xo's diagnostics) plus the
last line, so GNU as's three lines were dropped and the message said
"cc failed".

## Fix

gen.xo emits the directives only when `!g.x86`; firstErrors also keeps
`Error:` and `error:` lines. Seeds built before the fix still carry the
directives, so scripts/ci/suite.sh exports `XO_CC=clang` on x86 linux
runners (clang's assembler accepts them). Diagnosing a runner without
ssh: scripts/ci/cc-probe.sh, run ahead of the xo build by suite.sh's
"before xobuild" rule.

## Rule

A failure message that names the tool and not what it said hides the
cause; keep the tool's stderr. Anything the native backend writes into
assembly text is read by three assemblers (clang, GNU as on arm64 and on
x86, and Xo's own); a directive one of them rejects must be guarded by
the architecture it is for.
