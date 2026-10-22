# 0341: Runtime record layouts are declared once, and copies are checked

Status: Proposed (owner request, poll item 3, 2026-10-22; Xo Lang 2)

## Context

The native runtime (std/native/llrt) reads and writes its records (the
context, the pool and worker records, wait list entries, a Mutex, a
poller descriptor) through byte offsets: `rt.mem_load(c, C_WENT, 8)`.
Many modules need another module's offsets and cannot import it (an
import cycle: rtstats, tasks, pool, poll, and mutex sit below or beside
each other), so they copy them by hand. On origin/main at 2026-10-22
10:00, 2,332 integer constants in llrt carried 1,833 names; 274 names
were defined in two or more modules (773 copies), and 87 of those names
had different values. Most of those differences are distinct records
that happen to share a name (a channel's entry E_NEXT 56, a wait list's
16) or per-OS values (EAGAIN 35 on darwin, 11 on Linux), but nothing
tells them from drift:

- fault.xo's `C_TRACE = 320` is ctx.xo's `C_FAULT_TRACE`, while ctx.xo's
  own `C_TRACE` is 344 (and pool.xo copies that one as `C_TRACE = 344`).
- pool.xo's `N_CANCELED = 4` is a word index where budget.xo, tasks.xo,
  twrun.xo, and dump.xo use the byte offset 32 under the same name.
- 2026-10-22: the Mutex yield (decision 0118's addendum) needed a word in
  the deadlock counts record; `DLK_SIZE` in ctx.xo had to change by hand
  with pool.xo's new `DLK_MNOYIELD`, and a miss would have written past
  the record with no error.

## Decision

1. **One home for shared layouts.** A leaf module,
   `std/native/llrt/layout.xo`, which imports nothing but native/rt,
   declares every record layout that more than one module uses, as
   `pub const`, grouped by record with the record's size last
   (`CTX_SIZE`, `POOL_SIZE`, ...). Modules use `layout.C_WENT`; the
   record's owner keeps only the layout of fields no other module
   touches. A word index (for the slot atomics) is the byte offset's name
   with `_W` and is declared next to it.
2. **Migration is incremental.** New shared fields go to layout.xo from
   now on. Existing copies move when their module is next changed, a
   record at a time (ctx first, then pool and worker, then the wait list
   entries), each move a no-behavior-change commit checked by the
   output identity of compiler/xo and the runtime goldens.
3. **Copies that remain are checked.** Until a copy moves, it carries a
   marker on its line naming its source: `const C_TRACE = 320 // = ctx.C_FAULT_TRACE`.
   A repository test, TestRuntimeLayoutCopies (internal/repotest), parses
   llrt's integer constants and fails when a marked copy's value differs
   from its source's, and when a module defines a constant with the name
   of another module's `pub const` and a different value without a
   marker (a likely stale copy or a misleading name, like fault.xo's
   C_TRACE). Distinct records that share a name get distinct names or a
   `// own:` marker that says the name is local.
4. **Sizes are checked where records are made.** A record whose size
   changes adds its new field and the size together in layout.xo; the
   test also fails when a record's fields (by their prefix) reach past
   its size constant.

## Consequences

- Drift between copies becomes a test failure in the fast repo checks
  (pushq runs TestCodegenInputsClassified and friends; this joins them).
- layout.xo is one more module in every llrt build; it holds constants
  only, so no code and no call.
- The migration touches many files over time; done record by record with
  identity checks, it changes no output.
- Readers of a module see `layout.X` instead of a local constant: one
  lookup further (`xo def layout.X`), and no copy to trust.

## Alternatives

- **Record types for runtime records** (a `#[layout]` struct the
  compiler lays out, with field access compiled to loads and stores):
  the clean end state, but a language and backend feature for runtime
  code only, with every backend's lowering and the C ABI to match.
  Worth a later ADR; this one makes the current code safe now.
- **Generate the copies** from one table (scripts/gen): removes hand
  edits but keeps 773 copies to read and adds a generator to run; the
  leaf module removes the copies instead.
- **A check alone, no home**: catches drift but keeps the copies, the
  import cycle workarounds, and the naming collisions.
- **Status quo**: drift found by crashes; the DLK_SIZE miss above would
  have been a heap overrun in a record allocated at start.
