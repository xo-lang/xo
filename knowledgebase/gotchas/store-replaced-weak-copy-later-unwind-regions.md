# A replaced weak copy's later unwind regions keep their compact entries

Status: Observed (Xo Lang 1, 2026-11-15, found by aws-43 in the MySQL driver)

## Symptom
`xo test --std-module=native/llrt/pg --backend=native` on darwin/arm64, with std's
store present: `xo: link: prog-darwin.45.o: no FDE for the DWARF mode function at
4295777920` (one of std's store parts). The same tree linked with a fresh
`XO_BACKEND_CACHE_DIR` (no store yet, so a whole program build) and with a cached test
binary, so it looked like a stale cache. It appeared when `fn(b) { sha1.sum(b) }`
replaced `sha1.sum` in std/native/llrt/mysqlauth.xo.

## Cause
The program defined `mysql.handshake` strongly, so the store's weak copy was replaced.
compiler/macho store.xo uncoalesced_unwind drops a replaced copy's compact unwind
entries, but it matched only the copy's start address. handshake has a second unwind
region, its fault stubs. With a call site there the region names a personality, which
puts it in DWARF mode (cfi.xo compact_encoding). Its entry stayed. The region's FDE
names `handshake` plus an offset, which resolves to the program's copy, so no FDE starts
at the entry's address. The closure only changed handshake's stubs enough to need the
personality.

## Fix
uncoalesced_unwind drops every entry inside a replaced definition's extent, from its
address to the next symbol of its section. The error now names the nearest symbol below
the address and the entry's relocation target (unwind.xo names_at, cu_target), since a
nameless address cost a search. The regression test is compiler/macho atoms_test.xo, "a
store link drops every unwind entry of a replaced weak copy". ELF and PE store links were
checked the same way on 2026-11-15 and are not affected. ELF FDEs point at the copy
section-relatively. PE keeps each copy's .pdata at the copy's own address, and drops it
with a dead copy.
