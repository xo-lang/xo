# macOS debug info: Go and Xo against Apple's tools (2026-10-22)

`run.sh XO OUT` (results.txt is one run: Go 1.26.1, macOS 27.0.1,
Apple LLVM 21, xo at main `46a1ed503`, the Mac's arm64). Source for the
post "Where macOS keeps a program's debug info" (website draft).

| Build | Binary | DWARF | atos on main.greet / greet | dsymutil |
|---|---|---|---|---|
| Go default | 1,749,250 B | `__zdebug_*` in the executable (compressed) | `main.greet (in hello-default) + 0`: no file or line (results.txt:23) | warning: no debug symbols in executable (results.txt:42) |
| Go `-ldflags=-compressdwarf=false` | 2,888,578 B | `__debug_*` in the executable | `main.greet (in hello-plain) (main.go:4)` (results.txt:56) | the same warning (results.txt:80) |
| Xo (today, decision 0239) | 706,242 B | hello.dSYM beside it, 1,404,699 B (results.txt:126) | `xo_f_greet (in hello) (main.xo:3)` through the dSYM (results.txt:98) | not needed: xo writes the bundle |

lldb `image lookup -v -n`: Go default finds the symbol but no
LineEntry or CompileUnit (results.txt:30 to 37); uncompressed gives
`main.go:4` (results.txt:72 to 75); Xo gives `greet at main.xo:3:3`.
dwarfdump --debug-line on Go's default prints an empty table.

Other formats (the same programs cross built): Go's linux/arm64 ELF
keeps `.debug_*` names but compresses each section in place (an
Elf64_Chdr with ch_type 1, zlib, then the 78 01 zlib header); Go's
windows/amd64 PE has `.zdebug_*` sections (DWARF, not a PDB). Xo's
linux/arm64 ELF carries uncompressed `.debug_*` in the executable.

Apple's own model, for contrast (a C hello, `clang -g -O0`, linked by
ld): the executable has no debug section; its debug map (`nm -ap`, an
OSO stab) names hello.o, where the DWARF stayed; dsymutil collects it
into hello.dSYM (9,224 B), and with hello.o deleted atos still answers
`greet (in hello) (hello.c:4)` from the bundle (results.txt:175 to 189).
