# 0341: Runtime record layouts are declared once, and copies are checked

Status: Accepted (owner, 2026-10-22). Proposed the same day (owner
request, poll item 3; Xo Lang 2). Step 1 implemented: the context record.

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
   `pub const`, grouped by record with the record's size last, named by
   the fields' prefix (`C_SIZE` for the context's `C_*`). Modules use `layout.C_WENT`; the
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
   of a layout.xo constant and a different value without a marker (a
   likely stale copy or a misleading name, like fault.xo's C_TRACE). A
   local constant that only shares the name gets a distinct name or a
   `// own:` marker. The name rule covers layout.xo's names only: against
   every module's `pub const` it flagged 96 constants on 2026-10-22,
   nearly all legitimate (per OS values such as EAGAIN, TLS states such
   as DONE, unrelated R_SIZE and CAP), so its reach grows as records move.
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

## Step 1 (2026-10-22): the context record

std/native/llrt/layout.xo holds the context record's fields (C_NODE to C_LOCKS) and C_SIZE
(moved from ctx.xo, which keeps `pub const C_X = layout.C_X` for its
users). The 14 copies that import cycles had kept (fault.xo, pool.xo,
twrun.xo) use layout.xo now; fault.xo's C_MSG and C_TRACE, second names
for C_FAULT_MSG and C_FAULT_TRACE, take the fields' names. h2srv.xo's and
netconn.xo's C_SIZE (their connection records) are marked `// own:`.
Code identical: one xo, main's std against the moved std at the same
path, every 12th golden and compiler/xo (27 programs) by disassembly,
the same except the line numbers in the std source locations the
programs embed (ctx.xo:323 became ctx.xo:284); a control with main's std
on both sides, 27 of 27 identical
(~/xo-lead/notes/Xo Lang 2/lay-ident3.sh). scripts/identity does not fit
a std-only change: it builds both sides with one std. TestRuntimeLayoutCopies is in
internal/repotest.

## Step 2 (2026-10-22): the deadlock counts record

The ten words DLK_ROOTS to DLK_MNOYIELD (word indices for the slot
atomics) and DLK_SIZE (80 bytes) moved to layout.xo from pool.xo (their
owner, which keeps them as aliases) and ctx.xo (copies of DLK_ROOTS and
DLK_SIZE, the miss that started this decision). Code identical by the
step 1 method (lay-ident3.sh on this tree, line numbers in the embedded
locations normalized): 27 of 27 programs the same by disassembly; the
arm64 runtime goldens (task, chan, conc, mutex, fault, scope, select,
panic, isolate) pass.

## Step 3 (2026-10-22): the pool and worker records

P_MU to P_WS, P_SIZE (the pool record) and W_NEXT to W_SIZE (the worker
record, with its word index names W_*_I and W_*_W) moved to layout.xo from
pool.xo (which keeps them as aliases). The copies in ctx.xo (P_DLK,
P_SIZE), poolsetup.xo (nine), sanhook.xo (W_HOOKS, W_POOL, P_FIBERS),
sched.xo, mutex.xo and tasks.xo use layout.xo; tracerec.xo's W_CHUNK, a
second name for the worker's trace field, is layout.W_TRACE. Three
constants that only share a name are marked `// own:` (hclient.xo P_IDLE
and P_SIZE, twrun.xo P_SIZE); two static word indices that shared a worker
field's name are renamed (testworld.xo W_RNG to W_TWRNG, tracerec.xo W_TRACE
to W_TRACEREC: TestStaticWords wants the number on the const's line).
Checked as step 2, except that the disassembly comparison also ignores the
literal pool offsets that follow a location string whose line number changed
length (the same instructions; `add x3, x3, #0x9c9` against `#0x9ca`):
26 of 27 programs identical; json_wire_names differs in one `adrp` page
(46 against 47), the data after the shifted strings crossing a 4 KiB boundary.

## Step 4 (2026-10-22): the cancellation node

N_PARENT to N_MU, N_CANCELED (byte offset 32), N_CANCELED_W (its word
index 4, which pool.xo had named N_CANCELED while its other users used
the byte offset under that name) and N_SIZE (56) moved to layout.xo from
pool.xo. dump.xo, budget.xo, tasks.xo, twrun.xo and ctx.xo (N_BYTES) use
layout.xo; pool.xo's N_CANCELED_AT, a second name for the byte offset,
is N_CANCELED, and its word uses are N_CANCELED_W. twrun.xo allocates 64
bytes for a node, not 56 (a margin from the C runtime): that constant,
N_SIZE there, is N_ALLOC now, the same value. Checked as step 3: 27 of 27 programs identical.

## Step 5 (2026-10-22): the wait list and its entry

WL_FIRST, WL_LAST, WL_SIZE (xo_wl) and E_PARK to E_GRANTED, E_WRITER_W,
E_SIZE (xo_went, the context's wait list entry) moved to layout.xo from
tasks.xo (the owner) and mutex.xo (copies, with hserve.xo's two). chan.xo's
E_* are xo_cent's, a different record: its E_PREV and E_NEXT, which share
names with this one's but not values, are marked `// own:`. Checked as
step 3: 27 of 27 programs identical.

