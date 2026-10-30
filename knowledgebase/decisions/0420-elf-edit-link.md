# 0420: the ELF edit link (prepare cache, one walk, std's half next)

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

## Consequences

- The ELF edit link loses the second walk's cost. Native linux arm64 host, interleaved n=30, A is the link with undefined() once, B adds this walk change:
  - HTTP: 48.61 to 38.31 ms (0.788; resolve 17.85 to 7.94 ms);
  - fib: 23.40 to 20.20 ms (0.863; resolve 7.79 to 4.79 ms).
  Off, miss and hit are byte-identical in both. The prepare cache (E2) alone measured within Docker's noise.
- Linux edit binaries can keep a few std sections that the second walk would have dropped: 0.004% on xo, under the lead's bound of about 1%.
- toolchain.md says that a linux edit build's store link is not promised byte-equal to the parts link.

## Alternatives

- Keep the second walk and make it cheaper: roots only where program code is reached, and a DWARF cut cached per live set. Same bytes, more code, a smaller gain.
- Skip to the half (E4), which removes resolve and most of prepare. It is larger, and its gain depends on the same walk question.
