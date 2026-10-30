# 0420: the ELF edit link (prepare cache, one walk, std's half)

Status: Accepted (lead, 2026-10-29: option 1 of the second walk; owner approved the ELF port the same day)

## Context

Decisions 0388, 0401 and 0409 made darwin/arm64's separately compiled edit link fast:
- a prepare cache;
- a debug map in place of std's DWARF;
- std's half reused.
The owner approved porting them to linux arm64 and amd64 on 2026-10-29. The ELF store link (compiler/macho elfstore.xo, elflink.xo) is measured first, in Docker on the Mac:
- HTTP edit link 52 to 57 ms (prepare 16, resolve 23, finish 15);
- fib 28 to 30 ms.
That is 2.3x darwin's before 0409.

A CPU profile of 20 HTTP links (1 kHz) shows:
- gc_sections is 19% (its dw_strip 12%). The store keeps every SHT_NOTE section whole (elf_whole), so std's objects carry .note.xo.atoms, and the link's second walk walks all of std again and cuts std's DWARF again. The prepare walk had already chosen std's live atoms;
- undefined() is 11.6%: five walks of every live symbol a link;
- def_of 9.4%, StoreView.cut_with 9.3%, debug_sections 7.3%.
The prepare cache (E2) alone gave nothing measurable.

## Decision

1. The ELF store link runs in steps (prepare, resolve, layout, write), with laps and XO_STD_STORE_REPEAT, as darwin's (E1), and keeps a prepare cache (E2: prepcache.xo's Prep with live and dw_dropped, keyed by macho.elf_prep_key).
2. undefined() is computed once in synthesize_start and once in collect_imports. The output is unchanged.
3. std's store objects are whole for the second walk: they are roots, keep their sections, and get no dw_strip. Their live set and DWARF cut are the prepare walk's, which is darwin's semantics.
   - Off, miss and hit stay byte-identical.
   - Compared with the second walk, a std section that only a dead program function reaches stays in a separately compiled edit binary, as on darwin. Measured on the native linux arm64 host, 2026-10-29: fib is unchanged (1,944,544 bytes); HTTP grows by 136 bytes (6,959,704 to 6,959,840); xo grows by 4,160 bytes (110,898,912 to 110,903,072, 0.004%).
     By section, the kept std code is the only growth:
     - HTTP: .text +56, .symtab +48, .strtab +35 bytes;
     - xo: .text +1,208 (0.0017% of 70,831,348), .symtab +1,200, .strtab +1,724, .debug_line +14, .debug_loclists +9, .gcc_except_table +8 bytes.
     The separate compilation sample (scripts/docker-sepcomp-sample.txt, 26 goldens, run natively on the linux arm64 host) gives the same exit status and output separately compiled and whole.
   - In decision 0388's XO_STD_STORE flow (XO_STD_ROOTS=1, XO_DCE=off), XO_STD_STORE_CHECK=1 still finds the store link byte for byte the parts link, on fib, HTTP and xo, with and without this change. So the two are not promised equal in general (edit builds differ), but that check keeps passing.
4. Next: std's half on ELF (E4), after this measures. ELF specifics: std's .rodata and .text first, with option B's four PT_LOADs in edit builds; RELA needs almost no restore; GOTTPOFF and weak-call rewrites restore their opcode bytes. Then debug info for the edit binary (E3), std's relocated DWARF in the half.

5. E4 and E3 as built (2026-10-30): std's half on ELF, decision 0409's design.
   - Layout (ElfLinkOpts.split_prog): std's .rodata, std's .text, then the program's read only data (.rodata.xoprog) with the dynamic and unwind tables, the program's code (.text.xoprog) with the PLT, then the data: five PT_LOADs in place of three (elflink.xo out_order).
   - Capture (elfhalf.xo capture_half, after a full split link): each of std's sections in .rodata and .text (not one a __start_/__stop_ pair bounds) holds the link's output bytes and keeps only the relocations whose value can change: GOT and thread local forms, imports, the program and the start object, commons, targets outside std's .rodata and .text. std's DWARF the same way (E3): its chunks come first in each joined .debug_* section, so their offsets are fixed. arm64 relocations write whole fields and keep the instruction's other bits, so no original bytes are put back; x86-64's read the opcode before the field (a branch's e8 or e9, GOTTPOFF's REX, opcode and ModRM), which halfcore.xo half_restore puts back. Also kept: std's globals, COMDAT groups and undefined names (before and after the strip).
   - Key and cache: <backend cache>/stdhalf/sha256("elf", the ELF prepare file's name, interp, entry, static, strip, release, debug_link), kind stdhalf, pruned with the cache.
   - A hit (elf_store_prepare_half, resolve_half): std's objects are views of the file, the program is parsed once, the half's globals, groups and undefined names stand for std's, then the start, the strip and the imports as usual; the pin check (std's .rodata and .text addresses, phnum) after layout.
   - It falls back to the full link (whose half replaces the entry) when the program defines (not weak) a name std defines or has a common symbol, or the pins differ. A full link whose program defines a name std defines writes no half (ElfStoreLink.shadows; darwin's StoreLink.shadows, the same day).

## Consequences

- The ELF edit link loses the second walk's cost. Native linux arm64 host, interleaved n=30, A is the link with undefined() once, B adds this walk change:
  - HTTP: 48.61 to 38.31 ms (0.788; resolve 17.85 to 7.94 ms);
  - fib: 23.40 to 20.20 ms (0.863; resolve 7.79 to 4.79 ms).
  Off, miss and hit are byte-identical in both. The prepare cache (E2) alone measured within Docker's noise.
- Linux edit binaries can keep a few std sections that the second walk would have dropped: 0.004% on xo, under the lead's bound of about 1%.
- toolchain.md says that a linux edit build's store link is not promised byte-equal to the parts link.
- std's half (item 5), native linux arm64 host, interleaved n=30, HTTP edit link medians:
  - the half (A: the split layout with no half, B: the half): 36.75 to 22.57 ms (0.614; prepare 14.82 to 6.82, resolve 7.50 to 4.63, finish 14.46 to 10.91);
  - parse once, std's undefined names and DWARF from the half (A: the half before them): 22.54 to 16.48 ms (0.731; prepare 7.05 to 5.32, resolve 4.79 to 2.14, finish 10.80 to 9.00).
  - linux/amd64 (the owner's WSL2 host, n=30; A: the split layout with no half, since x86-64 had none before this; B: all of item 5): 60.10 to 39.27 ms (0.653; prepare 22.82 to 17.80, resolve 9.97 to 4.23, finish 26.34 to 17.23), with the same checks passing.
  - the whole of item 5 with the half on by default, against the parent's xo (linux arm64, n=30): 39.07 to 13.84 ms (0.354; prepare 15.80 to 5.32, resolve 7.96 to 1.96, finish 15.39 to 6.53).
  The half drops 189,989 of std's relocations and keeps 2,606 (arm64). Off, miss and hit are byte-identical for the 26 programs of the separate compilation sample, fib and HTTP; each runs as its build without the split; an edit linked from the half equals the same edit linked in full; tracebacks and XO_PROF read the split binaries. A half file per prepare key is about 32 MB (HTTP) and 26 MB (fib), std's DWARF most of it.

## Alternatives

- Keep the second walk and make it cheaper: roots only where program code is reached, and a DWARF cut cached per live set. Same bytes, more code, a smaller gain.
- Skip to the half (E4), which removes resolve and most of prepare. It is larger, and its gain depends on the same walk question.
