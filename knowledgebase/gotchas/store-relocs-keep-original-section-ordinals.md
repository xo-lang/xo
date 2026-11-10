# A store object's section relocations keep the original section ordinals

Status: Observed (Xo Lang 2, 2026-11-10, decision 0596)

## Symptom
`xo test --std-module=native/http` with std's test variant defining every function weak:
`xo: link: prog-darwin.17.o: no FDE for the DWARF mode function at ...` (the object is one of
std's store parts, `store.parts/prog-darwin.17.o`).

## Cause
The program defined `testing.fakenet_handle` strong; the store's weak copy stayed live, its
compact unwind entry named the store copy while its FDE's pointer resolved by name to the
program's copy. compiler/macho store.xo uncoalesced_unwind drops such entries, but ran only for
program objects, and matched an entry's function by (section ordinal, address): a compacted
store object's symbols carry renumbered ordinals (keep_syms, `ord`), while its non-external
relocations keep the original section ordinals, so nothing matched.

## Fix
Run uncoalesced_unwind on store objects too, and match by address alone (a Mach-O object's
sections do not overlap). When comparing a store object's symbol with a section relocation,
compare addresses, never ordinals.
