# 0228: Fault entry points as Xo exports the emitters call

Status: Accepted, implemented (2026-10-14, B06 part 2 of the zero-C
audit; std/native/llrt/fault.xo; measurements in
bench/results/fault-record-2026-10-14.md, "Part 2").

## Context

Generated code reports a failed check (overflow, division by zero, an
index out of range, a shift, negation, abs, a Float to Int conversion,
`fault(msg)`) by calling a runtime entry point: `xo_fault_overflow`,
`xo_fault_index`, and seven others, and `xo_fault`.
- Before this decision they were C in xo_rt.c (and xo_bytes.c for
  Bytes). Each called a cabi.xo export to write the message into a C
  stack buffer, then fault_text, which recorded the fault and raised it.
- B06 part 1 moved the record itself to Xo (fault.xo).
- Decision 0226 (list objects) and decision 0221 point 7 (the
  personality) set the pattern for removing such a C level: the emitters
  name the Xo export, and the C entry point stays as a forwarder only
  for code from an older xo, which the seed builds the next xo with.

## Decision

1. **The entry points are Xo exports**, defined in cabi.xo:
   `fault_raise_overflow`, `fault_raise_shl`, `fault_raise_shift`,
   `fault_raise_divzero`, `fault_raise_neg`, `fault_raise_abs`,
   `fault_raise_index`, `fault_raise_bytes_index`,
   `fault_raise_float_int`, and `fault_raise_text` (`fault(msg)`: the n
   bytes at p). Their symbols are `xo_x_fault_raise_*`, and the location
   is an Int, the address of the zero terminated location.
   - fault.xo `raise_*` writes the message (the texts cabi.xo's writers
     had) into the thread's message buffer.
   - That buffer is 1,024 bytes of unwind.xo's per thread scratch record
     (`unwind.msg_buf`). A longer `fault(msg)` text goes to a malloc'd
     block that is never freed, as the C's was.
   - It then calls the C's `fault_raise_msg` through `rt.call_c2`, which
     records the fault in the current context (creating one if the
     thread has none) and starts the unwind.
2. **The raise address**: `rt.static_word` 14 holds the address of
   fault_raise_msg. The C sets it with the export `fault_init`, next to
   the heap words: first in main, or in the XO_RT_NO_MAIN constructor.
3. **The emitters**:
   - selfhost/arm64 isel.xo `fault_entry` maps a C entry name to its
     export (`set_fault` and the `fault(msg)` call).
   - selfhost/llvm (call.xo, func.xo) calls the exports defined in the
     module, passing the location as `i64 ptrtoint (ptr @.loc.N to i64)`
     and `fault(msg)`'s data through a `ptrtoint`. The C declarations in
     the module head stay unused, so the head is unchanged.
   - All targets: the exports exist in every native program.
4. **The C entry points forward** (`xo_fault_*` in xo_rt.c and
   xo_bytes.c, three lines each) for code from an xo whose emitters name
   them. Delete them once a seed's emitters name the exports.

## Consequences

- The message writers left cabi.xo (`fault_overflow` and the others,
  which wrote into a C buffer); xo_rt.c no longer declares them.
- Faults raised from runtime C (fault_text: budgets, xo_fault_at, HTTP
  stubs) still go through fault_raise_msg in C.
- bench/perf/fault on the Mac: 0.962 to 1.036 of main's instructions.
  isolate on arm64 is 1.036, about 500 instructions more per fault (the
  export's frame and the copy into the message buffer).
- arm64 asm goldens: the 10 fault calls of fib and sieve now name
  `xo_x_fault_raise_*`.

## Alternatives

- **Keep the C entry points** (the forwarders as they are): a C level on
  every fault, and the forwarders would never go.
- **Format into obj.xo's heap record scratch** (B02): a thread that has
  not allocated yet has no heap record, and a fault can come first.
  unwind.xo's scratch is mapped on demand by the faulting thread itself.
- **Raise from Xo without C** (fault.set, then the unwind directly):
  needs the C's ctx() (which creates a context on a bare thread),
  rt_fault_leave (the real-time section drain), and the personality's
  address. These stay C with fault_raise until B06's remaining glue
  moves.

## Addendum 2026-10-25: the location as a file, line, and column

The location argument was the address of a `.asciz` "file:line:col", one
per checked operation: 562 strings and 18.0 KB in the release hello, 2,917
and 94.2 KB in CRUD, 771 of CRUD's repeating the same std/native/llrt
path.

The emitters now split the position (ssa.pos_split) and pass the file
name, the line, and the column: a per part deduped `.asciz` for the file
name (33 in hello, 83 in CRUD), an `adrp`+`add` for it, and one `movz` per
number in the stub. `cabi.xo`'s entries take them after the operation's
own arguments; `fault.xo` `set_pos` makes "file:line:col" in one heap
block where the fault is recorded (the raise does not return). The second
raise export is `fault_raise_pos`, reached through `rt.static_word` 90
(`fault_init_pos`), so `fault_raise_msg`'s two-argument C call stays as it
is: the qsort comparator in the fault tests calls it with (message,
location).

`fault(msg)` and contracts (`b.fault` lowers to `fault_raise(text, loc)`)
keep the old two-argument location; a fault whose position is not
`file:line:col` (line 0) is printed as its whole text.

Costs and checks: the stub gains two instructions per checked operation
and loses no relocation; the location's text is no longer duplicated per
operation. The format is byte for byte the same, which the fault goldens
check on arm64, amd64, and LLVM (bytes_oob, fault_unwind, fault_audit,
lsr_fault_neg, and the others). The function and part cache versions are
bumped (v8, v4); the old selfhost IR dump replays that call the
three-argument exports are stale from here (decision 0217 keeps those
dumps frozen, so they are not re-recorded).

Measured (bench/results/fault-locations-2026-10-25.md, darwin/arm64
release, the same source tree): hello's `__cstring` 17,662 to 1,278 B and
CRUD's 88,910 to 5,485 B, `__text` 172,492 to 179,660 and 1,412,032 to
1,444,720 B (11 to 13 B a site), and the binaries 5.5% and 2.5% smaller.
Every arm64 run golden, the 49 LLVM goldens that print a fault, the
linux/amd64 fault goldens and Docker sample, the slice fault corpus, and
the fixed point pass with the same stderr.

