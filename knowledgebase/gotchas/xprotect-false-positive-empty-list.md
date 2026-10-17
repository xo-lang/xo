# XProtect flagged compiler/macho programs as malware (MACOS.SOMA.AUENB)

Status: Observed (2026-10-16), fixed

## Symptom

macOS showed "Malware Blocked: 'http_stream' was not opened because it
contains malware" for an Xo built arm64 program, and the exec waited on
the dialog (the process asleep at `_dyld_start`; see
gotchas/amfi-gatekeeper-macho-layout.md for how such a wait looks). A
yara scan with the Mac's own rules (the current ones are in
/private/var/protected/xprotect/XProtect.bundle/Contents/Resources/XProtect.yara,
version 5318 here; the copy in /Library/Apple/System/Library/CoreServices/
is older) matched 3 of the 179 internal/llvm goldens linked by
compiler/macho (http_stream, net_only, http_conn_fault), none linked by
cc and none built by LLVM:

```
XProtect_MACOS_SOMA_AUENB http_stream
0x24c0c8:$b0: 6B 01 22 91 CC 09 00 90 8C 01 22 91
```

## Cause

The rule's `$b0 = { 6b 01 22 91 cc 09 00 ?0 8c ?1 ?? 91 }` with
`filesize < 5MB` (its `or` is apart from the Mach-O condition) is
`add x11, x11, #0x880; adrp x12, page; add x12, x12, #imm`: an arm64
address load into x11 of a symbol at page offset 0x880, then one into
x12. In compiler/macho's layout __DATA began with xo_sys_words (0x80
bytes) and xo_static (0x800 bytes since decision 0237's 256 words), so
the immortal empty list was always at page offset 0x880, and code that
loads it into consecutive registers (tls13.retry, tls13.dummy_ccs: list
fields set to empty) formed the pattern. ld64 orders the data otherwise.

## Fix

The arm64 emitter writes the empty list and the zero handle first in
the data (compiler/arm64 gen.xo end_data), at page offset 0.
internal/arm64 TestEmptyListAtPageStart checks the offset on every Mac;
TestXProtectRules scans those three goldens, linked by macho and by cc,
with the Mac's rules: with yara when it is on PATH (`brew install
yara`), else in scripts/docker-linux.sh's container (its image carries
yara since 2026-10-16) when Docker runs; XO_XPROTECT_DOCKER=off skips
the container.
Any address at page offset 0x880 loaded into x11 and then x12 can still
match; scan new layouts with the rules.
