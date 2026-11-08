# 0575: a resident runner that loads xo's objects into a running process

Status: Accepted (owner, 2026-11-08, on the lead's recommendation, as a later stage of 0572). Proposed (Xo Lang 2, 2026-11-08; the owner decides). Design
only: no code until the owner decides 0572 (the REPL), which this
serves first. The lead asked for it after the MAP_JIT measurements, as
a stage of 0572 or its own ADR; it is its own because `xo run` and
`xo test` reruns could use it as well as the REPL.

## Context

On macOS the first exec of a newly built binary costs 146 to 251 ms.
This is the system's assessment of a new file, not Xo's code. It was
measured three ways (gotchas/darwin-new-binary-first-exec.md;
0572 Consequences):

- the Developer Tools listing does not remove it;
- an ad hoc re-sign does not (a new inode pays it again);
- a RAM disk does not.

Rewriting a binary in place skips it only when the size is unchanged.
A different size was killed (SIGKILL).

Copying code into MAP_JIT memory in a running process and calling it
cost 0.015 to 0.054 ms for 64 bytes, 0.33 to 0.35 ms for 1 MB and 2.0
to 2.6 ms for 8 MB. There was no assessment, and an unsigned or ad hoc
runner needed no entitlement.

The REPL (0572) rebuilds and reruns per entry. Linux runs a fib-sized
edit's new binary in 0.6 to 0.9 ms (0572's table, line 49); macOS adds
the first-exec cost on every entry. A resident process that loads each
entry's code would bring a Mac entry close to the Linux figure.

Xo already applies its own objects' relocations, in three linkers:
compiler/macho link.xo (Mach-O), elflink.xo (ELF) and pelink.xo (PE).
pelink includes pearm64.xo's ARM64 relocations as of 0569 stage 1. A
loader applies the same relocations at run time instead of link time.

## Decision

### Shape

`xo repl` (and later `xo run --resident`) starts one runner process per
session. The runner is an ordinary Xo program: the runtime, std and a
small loader. It reads entry objects from xo over a pipe, maps them and
calls them. xo keeps compiling. The runner only loads and runs.

### What the loader handles

- **Formats.** Mach-O arm64 first, since the Mac is where the cost is.
  ELF arm64 and amd64 follow from the same core. PE is not a goal: no
  first-exec cost was measured on Windows.
- **Relocations.** The kinds the linkers already apply:
  - PAGE21 and PAGEOFF12 (scaled for loads);
  - BRANCH26;
  - GOT_LOAD;
  - UNSIGNED 64 for data pointers;
  - SUBTRACTOR pairs for jump tables.

  DWARF relocations are skipped. One function takes the site, its bytes,
  the kind and the target's address, and both the linker and the loader
  call it. That is the shape pearm64.xo's a64_reloc already has: the
  macho linker's arm64 cases move behind it, and the loader reuses it.
- **Reach.** The entry's region is mapped within 128 MB of the runner's
  text when the system allows it. A branch out of range gets a veneer
  in the region, as link.xo's veneers do. Every external data reference
  goes through a GOT slot in the region.
- **Symbols.** An entry's undefined symbols resolve against two
  sources:
  1. the runner's own runtime and std, from a symbol table xo writes
     when it builds the runner;
  2. earlier entries' definitions.

  A later definition replaces an earlier one in the table, and old code
  stays mapped: v1 never unloads.
- **TLS.** The runner's image owns xo_tlocal. An entry is built so its
  thread-local accesses call the runtime's xo_tlocal_get/set/addr. This
  is the call form windows/arm64 already uses (0569), so an entry never
  defines TLS.
- **Globals.** The runtime's globals stay in the runner. An entry's
  module globals live in its region. A REPL binding is a global of the
  entry that made it, and later entries name it by symbol. 0572 pins
  plain-data bindings as literals, and the loader keeps the rest alive
  across entries.
- **Unwind.** Each entry's unwind data is registered: `__register_frame`
  for its __eh_frame on darwin and linux, with the LSDA for
  xo_x_personality. A panic or a fault then unwinds through entry code
  as through the runner's own.
- **Writing code on macOS.** The region is MAP_JIT memory, with
  pthread_jit_write_protect_np around writes and sys_icache_invalidate
  after them. The data is copied from a buffer: `read()` straight into
  MAP_JIT pages left them zero, which the measurement found.

### Containing a crash in an entry

- An entry runs on its own fiber. A fault on that fiber becomes a panic
  through the existing fault path (decision 0085's stubs). The runner
  prints it and keeps the earlier entries' state.
- Heap corruption, a scheduler deadlock and an `exit` cannot be
  contained. The runner then restarts and the REPL replays the session's
  entries. The rebuild-and-rerun model of 0572 stays the fallback and
  the reference, so the resident path only ever makes an entry faster.

### Stages (after the owner decides 0572)

1. Factor the arm64 Mach-O relocation cases into the shared function.
   No output change; the linkers' tests prove it.
2. Loader core over hand-built objects, with unit tests as
   pearm64_test.xo does: map, relocate, resolve, call.
3. The runner and its symbol table: one cached part's object loaded and
   called, then timed against a first exec of the same program.
4. The REPL behind XO_REPL_RESIDENT=1. It replays to the rerun path on
   any loader error.

## Consequences

- A Mac REPL entry costs its compile plus about a millisecond, not
  150 to 250 ms more than Linux.
- The three linkers and the loader share one relocation core, so a
  relocation fix lands once.
- Entries are built in a mode with calls for TLS and a GOT for external
  data. Their code differs slightly from a linked program's: a few
  percent at most on hot TLS paths, which a REPL entry rarely has.
- A loader bug cannot change a program's output. Any loader error falls
  back to the rerun path, and tests compare the two paths' output.

## Alternatives

- **Keep rebuild and rerun only.** The Mac pays 150 to 250 ms per
  entry. That stays the fallback.
- **Rewrite a fixed-size binary in place.** Same-size rewrites ran in 2
  to 11 ms, but any size change was killed. Padding every build to a
  fixed size is fragile, and it relies on a system behavior no one
  documents.
- **An SSA interpreter (0572 stage 3).** No macOS cost, but a second
  engine whose results can differ from compiled code. Kept as 0572's
  own option.
- **dlopen of a dylib per entry.** Not measured. A new dylib is a new
  file, so it is expected to be assessed like a new binary. Measuring it
  is stage 3's control.
