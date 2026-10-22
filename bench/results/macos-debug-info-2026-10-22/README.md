# macOS debug info: Go and Xo against Apple's tools (2026-10-22)

`run.sh XO OUT` on the Mac (macOS 27.0.1, Apple LLVM 21, Go 1.26.1,
arm64). Two runs: `results.txt` with xo at main `46a1ed503` (the DWARF
in a dSYM, decision 0239) and `results-0346.txt` with xo at main after
`ead761c21` (decision 0346 stage 1: the DWARF embedded, uncompressed;
the dSYM still written until stage 4). The Go and C parts are the same
in both. Source of the post "Where macOS keeps a program's debug info"
(website draft).

| Build | Binary | DWARF | atos | lldb `image lookup -v` |
|---|---|---|---|---|
| Go default | 1,749,250 B | `__zdebug_*` in the executable (compressed) | `main.greet (in hello-default) + 0`, no file or line (results.txt:23) | the symbol, no compile unit or line entry |
| Go `-ldflags=-compressdwarf=false` | 2,888,578 B | `__debug_*` in the executable | `main.greet (in hello-plain) (main.go:4)` (results.txt:56) | `main.go:4` (results.txt:75) |
| Xo before 0346 | 706,242 B (results.txt:88) | hello.dSYM, 1,404,699 B (results.txt:126) | `xo_f_greet (in hello) (main.xo:3)` (results.txt:98) | with the dSYM `greet at main.xo:3:3`; with it deleted the symbol only (results.txt:138) |
| Xo with 0346 stage 1 | 2,126,386 B (results-0346.txt:88) | `__debug_*` in a `__DWARF` segment of the executable (and a 1,409,148 B dSYM until stage 4) | `main.xo:3` | with the dSYM deleted `greet at main.xo:3:3` (results-0346.txt:138 to 144) |

dsymutil on either Go binary: `warning: no debug symbols in executable`
(results.txt:42, 80): it builds a bundle from a debug map, which Go
binaries do not have. dwarfdump --debug-line on Go's default prints an
empty table.

atos is not a test of where the DWARF is: with the Xo dSYM deleted it
still answered `main.xo:3` for the binary built before 0346
(results.txt:130), and the C run below did the same with the object
file gone; it answers from a symbolication cache keyed by the
executable. lldb reads the files each time, so the lldb column is the
check.

Apple's own model (a C hello, `clang -g -O0`, linked by ld,
results.txt:190 to 204): the executable has no debug section; its debug
map (`nm -ap`, an OSO stab) names hello.o, where the DWARF stayed;
dsymutil collects it into hello.dSYM (9,232 B).

Other formats (the same programs cross built): Go's linux/arm64 ELF
keeps `.debug_*` names but compresses each section in place (an
Elf64_Chdr with ch_type 1, zlib, then 78 01, zlib's header); Go's
windows/amd64 PE has `.zdebug_*` sections (DWARF, not a PDB). Xo's
linux/arm64 ELF carries uncompressed `.debug_*` in the executable.
