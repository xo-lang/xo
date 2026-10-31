# 0443: `xo dev size --by-cause`, native code size split by what made it

Status: Accepted (owner, 2026-10-30). Previously proposed (Xo Lang 2, 2026-10-30; the lead's queue item, moved
from Xo Lang 1, whose plan is ~/xo-lead/notes/lang1/plan-size-by-cause.md).

## Context

Code size has been measured as one number per push (xo's native
`__text`, CLAUDE.md "code size per push"), so a cause that grew slowly
was invisible. Xo Lang 1 found 11.8 MB of xo's 71 MB `__text` was a
fault exit building a zero arm64 `G` inline in every `var g: G` method.
It had hidden for three weeks: each push's delta was small. Its tw74 fix
takes xo from 71.0 to 62.5 MB.

Disassembly cannot split the text by cause:

- "after fault_continue" counts every cold block laid out after it, so
  it showed 35%;
- drop helpers have hashed names (`_xo_hKsB4YXRIq0`), so the RC bucket
  matched almost nothing.

The emitter knows the cause of every instruction when it makes it; the
assembler knows every instruction's bytes.

## Decision

1. **Causes.** A small fixed enum, extended only by adding codes:
   - `body`: everything not below;
   - `frame`: the prologue, epilogue and frame record saves;
   - `fault_exit`: the MIR blocks lowered from the SSA unwind exit block
     (`Func.unwind_exit`);
   - `landing`: landing pad blocks and their pad entry stubs (emit.xo
     pad_entry);
   - `fault_stub`: the out of line stubs of checked operations
     (bounds, overflow, division, nil) and their frame records;
   - `preempt`: the preemption poll stubs (decision 0286);
   - `rc`: retain, release and drop sequences, inline or as calls (the
     MIR made from the SSA rc ops);
   - `copy`: the moves of a struct or tuple value larger than a word
     (copy runs);
   - `inline_rt`: instructions whose inline site's callee is an
     std/llrt function;
   - `data`: bytes in `__text` that are not instructions (a function's
     local empty list, jump tables, alignment).
2. **Tags are labels, emitted always.** At each change of cause, the
   arm64 emitter writes a local label `L<prefix>_x<code>_dc<n>:`.
   - The text passes (pair_mem, reload_reuse, copy_runs and the other
     final line passes) pass it like the debug row labels `_dl`, `_dv`
     and `_du`: is_row_label takes `_dc`. So it changes no instruction,
     as those labels do not (gotchas/arm64-pair-mem-drops-row-labels.md).
   - A local label is in no symbol table, so the object and the binary
     are unchanged.
   - Emitting the tags always, with no switch, keeps the backend caches
     correct with no new key.
3. **The assembler counts.** compiler/macho's arm64 assembler, asked
   for causes (`assemble_causes`, the same parse and layout as
   `assemble`), returns rows of `function cause bytes`. In each text
   section, a function runs from its symbol to the next function
   symbol; it starts as `body`, and each `_dc` label switches the cause
   from its offset on.
4. **The tool.** `xo dev size --by-cause [--top=N] [--json] PROG|DIR`
   builds PROG for arm64 native (darwin or linux), with the backend's
   object cache read-through off, and assembles every part with
   `assemble_causes`. It prints:
   - bytes per cause, with the share of `__text`;
   - the top functions per cause;
   - totals per module, from each function's file.

   Also: `--json FILE` writes `{fn: {cause: bytes}}`, and `--diff A.json
   B.json` prints the per-cause and top per-function differences. The
   rows sum to the `__text` size read from the built binary, or the tool
   fails: a cause it cannot see is a bug, never a silent "rest". amd64
   follows in a second step (x86emit.xo writes the same labels; the
   amd64 assembler counts bytes, not words). LLVM is out of scope: it
   has no emit-time hook.
5. **Proof of no output change.** The tags' commit carries
   `scripts/identity` of the parent's xo against the new one (xo itself
   plus the arm64 goldens): byte-identical `__text`, and identical
   objects.

## Consequences

- Size has a ledger: a release compares the causes, a push claiming a
  size change shows which cause moved, and a slow growth shows as a
  rising row instead of hiding in the total.
- The emitted text grows by a label per cause change, a few per
  function. That costs assembler parse time, not output.
- Any new text pass must pass `_dc` labels like the other row labels
  (the gotcha above names them all).

## Shipped

arm64 (2026-10-30, Xo Lang 2): the emitter's cause labels (compiler/arm64
emit.xo cause_mark; isel.xo value_cause), compiler/macho causes.xo
(assemble_causes), XO_SIZE_CAUSES in compiler/xo asmparts.xo, and
`xo dev size` (compiler/cli/dev size.xo). Not yet: the copy cause (no
emitter site tags inlined copies yet; they count as body) and amd64
(its emitter writes no cause labels; XO_SIZE_CAUSES is ignored there).

First numbers, with a stage 1 xo (the checkout's bin/xo, built by the
older compiler): xo's rows cover 71,615,504 of its 71,616,076 `__text`
bytes (99.9%; 47 s): body 26.2%, landing 25.7%, fault_exit 22.0%, rc
19.9%, frame 2.7%, fault_stub 2.2%, inline_rt 0.7%, data 0.2%. A 320 KB
test program: body 62.5%, fault_stub 20.3%, frame 7.5%, landing 5.5%.
No output byte changes (scripts/identity of xo at the parent against
this change): linux/arm64 117 of 117 programs identical (offload host),
darwin/arm64 59 of 59 (every 8th golden, the Mac); fib and sieve's
whole assembly is the same with the cause labels removed.

## Alternatives

- **Disassembly with symbol heuristics** (Lang 1's prototype): too
  coarse, as above.
- **A cause field per MIR instruction summed in the emitter.** The
  final text passes (pairing, reload reuse, far slots, local empty)
  change instruction counts after the emitter, and the assembler
  expands some lines, so emitter counts would not sum to `__text`.
- **Tags only under an `XO_SIZE_CAUSES` switch.** The cached parts would
  then need the switch in their key, and the tags would rarely be
  exercised. Always-on tags are tested by every build and proven
  neutral once by identity.
- **A side section in the object.** It would change the binary's
  layout, and with it adrp offsets in `__text`.
