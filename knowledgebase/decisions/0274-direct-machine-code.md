# 0274: the native backends emit machine code, not assembly text

Status: Proposed (owner approval 2026-10-20, through the lead; Xo Lang 2).
Measured 2026-10-20; the plan reviewed by Xo lang 4 (2026-10-20: one Op
type for both architectures, explicit fault, fixed register and memory
operands, debug rows in the identity gate, `xo asm` unchanged) and by Xo Lang
3 (2026-10-20: what the line passes read from text, step 4).

## Context

The native backends select instructions as text templates (`add $d0,
$u0, #8`), the register allocator writes registers into the text, the
emitter joins each function's lines, a chain of passes rewrites those
lines (compiler/arm64 emit.xo: copy_runs_l, borrow_flag_peep_l,
reload_reuse_l, pair_mem_l, far_slots_l, far_tests_l, local_empty_l),
and the parts' text goes to Xo's assemblers (compiler/macho asm.xo,
enc.xo for arm64, enc_amd64.xo), which split it into lines again, parse
each operand, and encode. The per function back-end cache (decision
0220) stores a function's canonical text.

Measured (2026-10-20, darwin/arm64 Mac, xo at 9887fae6): a cold build
of xo itself with every cache off (`XO_BACKEND_CACHE=off XO_FE_CACHE=off
XO_PROF=cpu xo build --backend=arm64 --phase-times ./xo` in compiler/):
20.84 s wall, 104.68 s user, max RSS 5.19 GB; 8950 profile samples
(89.5 s of CPU at 100 Hz). Phases (wall): check 1.23 s, ssa 1.17 s,
insert_rc 5.21 s, inline 3.65 s, emit 7.03 s, write 0.15 s, assemble
1.01 s (80 objects), link 0.32 s. CPU, cumulative:

| where | CPU | of 89.5 s |
|---|---|---|
| arm64.G.function (selection to text) | 39.69 s | 44.4% |
| of it arm64.G.emit | 20.02 s | 22.4% |
| the line passes (copy_runs_l 4.05, reload_reuse_l 2.15, far_tests_l 0.77, pair_mem_l 0.71, local_empty_l 0.54) | 8.22 s | 9.2% |
| emit_inst (text of each instruction) | 3.59 s | 4.0% |
| labels (G.label) | 2.81 s | 3.1% |
| joining (raw.str_join) | 1.87 s | 2.1% |
| macho.assemble (parse 3.77, of it parse_line 2.85; sym 1.26, directive 1.13, word_fix 0.92) | 4.10 s | 4.6% |
| ssa insert_rc (Rc.rc_func) | 28.63 s | 32.0% |

Text costs about 12.4% of a cold build's CPU (assembler 4.10, building
and joining 7.0): what direct encoding removes outright. The line passes
(9.2%) shrink only if they move off text. Warm builds read cached text
and objects, so the gain is mostly cold builds and edits that touch many
functions.

What exists: enc.xo encodes every mnemonic and operand form the arm64
emitter writes (it assembles every native build since decision 0215),
enc_amd64.xo the amd64 emitter's; both are reached only through text
(Asm.instruction(mnemonic, operand byte slices)).

## Decision (proposed, for review)

1. A structured instruction, `Op`, between the allocator and the
   object: the allocator's output, the line passes' input and output,
   the encoder's input. One Op type for both architectures (a mnemonic
   id from a per architecture table, operands as a small tagged list),
   with per architecture encoders and printers. Operand kinds: an
   allocator slot (`$dN`, `$uN`, and their width), a fixed register
   (the scratch registers x16, x17, r11, xmm14, xmm15 that templates
   name, distinct from slots, so the passes' scratch liveness checks
   port), an immediate, a memory operand (base slot, index slot, scale,
   displacement, width, pre or post index), a shift or extension, a
   symbol with its relocation kind, and a label. An Op says outright
   whether it can fault and what its fault stub reads (Inst.has_fault,
   recover, fargs), not left to be read from text. Text is rendered from
   it only for `xo asm`, --asm, and debugging, through one printer.
2. The encoders split in two: parse (text to Op, the assembler's
   front, kept for `.s` input and the tests) and encode (Op to words and
   relocations). The direct path calls encode only.
3. Templates parsed once: selection keeps its text templates (the
   readable form agents edit), and each distinct template is parsed to an
   Op pattern once per build (a map by template text), its `$dN`/`$uN`
   slots filled by the allocator with register numbers instead of
   strings.
4. The line passes ported one at a time to Op lists, each behind byte
   identity: the object a build writes through Op must equal the one the
   text path writes, debug information included (DWARF line rows and
   their labels were the trap in copy_runs_l and borrow_flag_peep_l,
   gotchas/arm64-text-pass-scratch-live.md; the gate runs the
   TestDebugInfoSameCode style checks too), for every golden and the
   compiler (an identity script in the manner of
   scripts/arm64-identity.sh, comparing objects), with the text path
   kept until every pass is ported. The x86 MIR passes that rewrite
   template text before allocation (x86_mul_pair, x86_fold_loads,
   x86_sink_loads, x86_const_imm, x86_imm_into and others, which parse
   `OFF($uK)` and rename tokens by hand) become rewrites of Op patterns.
   What the line passes read from text today, which the Op form carries
   outright (Xo Lang 3, 2026-10-20, from lsr.xo and emit.xo):
   - labels by id, spelled per target only when printed (label_refs
     strips ELF's `.L` to match Mach-O's `L` today), in one label table
     whose reference counts include the LSDA's (reload_reuse_l and
     pair_mem_l count references in the text and in `ext`, the
     function's LSDA text: a landing pad referred to only from it is
     still a join point and is not dropped); the table is the first
     port's, since every later pass needs it;
   - debug row and variable labels as a marker kind that is never a join
     point, never counted as an instruction, and keeps its place before
     the instruction after it (borrow_flag_peep_l steps past them; g.dbgs
     is built from them before the passes);
   - the instruction count the range decisions use (far_tests_l,
     local_empty_l count lines starting with a tab but not `\t.`; comments
     and directives skipped), so the same far and near choices are made;
     a multi-line Inst.text is several Ops, and the passes that refuse
     such text today (lsr.xo's checks) refuse the group;
   - the operand forms the passes accept today and no more (`[sp, #k]`,
     not `[x29, ...]`; the `!` write back; borrow_flag_peep_l's `, cc`);
   - local_empty_l's literal placed after the function, as now.
   The order stays: copy_runs_l and borrow_flag_peep_l before the LSDA
   exists, then reload_reuse_l, pair_mem_l, far_slots_l, far_tests_l
   (last but one: every earlier pass changes instruction counts),
   local_empty_l. reload_reuse_l and copy_runs_l are ported first (6.2 of
   the 8.22 s).
5. Directives (CFI, DWARF lines and variables, LSDA, data) become calls
   on the object writer, not lines, a debug row attached to the Op it
   precedes so it moves with it in a defined order; the per function
   cache stores the Op list in a binary form instead of canonical text.
6. `xo asm` output stays byte for byte what it is through the
   transition (TestGoldenAsm and the asm goldens are how codegen changes
   are checked, and what agents read).
7. arm64 first, then amd64 (decision 0254's x86 templates); x86 text
   changes stay small meanwhile (Xo lang 4, 2026-10-20).

## Progress

- 2026-10-21, step 1's first part (the line pass speedups, branch
  direct-mc-stage1): label references counted once per function
  (lsr.xo label_counts) for reload_reuse_l and pair_mem_l; copy_runs_l
  and writes_scratch skip lines that cannot matter before slicing them.
- 2026-10-21, step 2 for arm64 (branch direct-mc-op): compiler/macho
  op.xo holds the Op (a mnemonic and typed operands: Reg, VReg, Imm,
  Shift, MemOp, Cond, Sym with its relocation, Raw for system registers,
  barrier options, fcmp's `#0.0`, and addv's `bN`), parse_op from the
  emitter's text, print_op back to it, and encode_op_fix: the word and
  the label fixup of every instruction the assembler takes, from typed
  operands. The assembler's text path is parse then encode; the per
  family text encoders are gone (enc.xo 1,680 to about 560 lines). Every
  instruction of the encoder's test table (427) round trips through the
  Op and encodes from it to the word clang's assembler writes. Next: the
  object identity of real builds (goldens and the compiler, darwin and
  linux, built with the old and the new assembler); then the emitter
  builds Ops directly instead of text (step 3), and amd64's assembler the
  same way.

- 2026-10-21, step 2 for amd64 (branch direct-mc-amd64): the amd64
  encoder already parsed each operand into XOp before encoding; op_amd64.xo
  makes the instruction a value of its own (XInst: the mnemonic with its
  lock or rep prefix, and the XOps), parse_x86_line, print_x86 back to AT&T
  text, and encode_x86_inst, which encode_x86 now calls. Every form of the
  encoder's tables round trips through the XInst to clang's bytes: the
  1,957 forms of AMD64_ENCODINGS (parse, print, encode both the printed
  text and the XInst) and the 3,192 of AMD64_SURVEY (LLVM's x86_64 output
  for the goldens).

- 2026-10-21, step 3 stage 1 for arm64 (branch direct-mc-step3): under
  XO_OP_VERIFY=1 the emitter parses each instruction line into its Op and
  fails the build when printing it does not give the line back (the Op
  keeps an explicit `#0` offset and a condition's spelling, `cs` as
  written); the line passes start moving to items (arm64 lines.xo: each
  instruction's Op beside its text): reload_reuse_ops and copy_runs_ops
  read their slot loads, stores, and word copies from the Op, and under
  XO_OP_VERIFY their output must equal the text passes', function by
  function. The text passes still produce the build's output. An
  XO_OP_VERIFY sweep of half the goldens and the compiler, darwin and
  linux (236 builds, the backend caches off), found no difference.

## Step 3 for arm64: design notes (2026-10-21, before the measurement)

What the text costs now, per function (compiler/arm64 emit.xo): selection
writes each instruction as a template rendered to text (emit_plain), the
line passes rewrite the function's lines in this order: copy_runs_l and
borrow_flag_peep_l (before the LSDA exists), label_counts (the label
table, by spelling, step 1), reload_reuse_l, pair_mem_l, far_slots_l,
far_tests_l, local_empty_l; the text is joined, and the assembler parses
it back (step 2: into Ops) to encode it. Step 3 removes the print and the
parse, and makes the passes work on structure instead of strings. The cold
build profile (phase times and XO_PROF of `xo build compiler/xo` with every
cache off; held 2026-10-21 for memory, the first item when it runs) is to
replace the 8.22 s and 6.2 s figures above and to say what each stage
buys.

The stages, each one pushed alone:

1. Ops beside the text. Each distinct template is parsed once a build
   into an Op pattern (a map by template text; `$dN`, `$uN`, `$wN` are
   operand slots), and emit_plain fills the slots with the allocated
   registers. The text is still what the passes read; under
   XO_OP_VERIFY=1 each Op is printed and compared with the line rendered
   from its template, and the first difference fails the build with both
   texts. Gate: XO_OP_VERIFY=1 builds of every golden and the compiler
   for darwin and linux arm64 with no failure; no output change at all
   (the object identity check, obj-identity.sh, every golden, and the
   fixed point).
2. A function's lines as items: an Op, a label by id (a join point, a
   debug row, or a variable location: the marker kinds of step 4's
   list), a directive (CFI, data) as a call recorded for the object
   writer, and raw text for what has no Op yet. The label table counts
   references from the Ops and from the LSDA's label operands, replacing
   label_counts' scan of text.
3. The passes ported one at a time, in this order: reload_reuse_l (the
   largest, and the one every later pass's input depends on), copy_runs_l,
   pair_mem_l, borrow_flag_peep_l, far_slots_l, far_tests_l,
   local_empty_l (far_tests_l counts instructions, so it stays after
   every pass that adds or removes one). For each: under XO_OP_VERIFY=1
   the text pass also runs on the printed items and its output must equal
   the ported pass's printed output, function by function (the first
   difference fails with both texts); the object identity check over
   every golden and the compiler, darwin and linux; the fixed point.
4. The assembler takes the items directly (encode_op_fix on each Op, the
   label table for the fixups): no join, no parse. Text is printed only
   for `xo asm`, --asm, and XO_OP_VERIFY, through print_op, so
   TestGoldenAsm and the asm goldens stay byte for byte (decision item 6).
5. amd64 the same way (decision item 7), after its assembler gets step 2.

What identity means at each stage: the same object (linux byte for byte;
darwin by disassembly or to the same output name,
gotchas/darwin-binary-identity-output-name.md) and the same `xo asm`
text, for every golden and the compiler, on both targets; a stage that
changes either is wrong, however good its output looks.

## Consequences

- About 11 s of 89.5 s CPU off a cold self build from steps 1 to 3 and
  5, up to about 19 s with the line passes on Ops; the emit phase's wall
  time falls with it (it runs on every core).
- It does not touch the largest cost: reference counting insertion
  (28.6 s, 32% of the cold build's CPU) is more than twice the text's, so
  this does not halve a cold build.
- Two forms of an instruction exist until the text path is retired:
  the printer must stay exact (`xo asm` output is what the identity
  tests and agents read).
- The cache formats change (fn cache version bump); the fixed point and
  identity gate every stage.

## Alternatives

- Keep the text and make the assembler faster: bounded by the 4.10 s it
  takes; building and joining stay.
- LLVM's MC layer: a C++ dependency; decision 0215 keeps the compiler in
  Xo.
- A per template parse cache only (step 3 without Ops): removes most of
  the parse, but the text is still built, joined, and rewritten.

## Measurement before stage 4 (2026-10-21)

The cold build profile the design notes asked for, after every line pass
had its item port (still checked only under XO_OP_VERIFY; the text passes
produce the output): `XO_BACKEND_CACHE=off XO_FE_CACHE=off XO_BIN_CACHE=off
XO_PROF=cpu xo build --backend=arm64 --phase-times ./xo` in compiler/, main
at 0ca5d4b8c, darwin/arm64 Mac. The machine was shared with other agents'
jobs (47.6 s wall, 149.9 s user, 4.69 GB max RSS; 133.9 s of CPU
sampled), so the shares below matter more than the seconds, which the
earlier quiet run measured lower.

| where | CPU | of 133.9 s |
|---|---|---|
| arm64.G.function (selection to text) | 58.83 s | 43.9% |
| of it arm64.G.emit | 25.27 s | 18.9% |
| macho.assemble (parse 7.48: parse_line 5.65) | 8.24 s | 6.2% |
| copy_runs_l | 5.69 s | 4.2% |
| emit_inst (text of each instruction) | 4.27 s | 3.2% |
| reload_reuse_l | 2.27 s | 1.7% |
| scan.split (splitting text into lines) | 1.91 s | 1.4% |
| G.label | 1.86 s | 1.4% |
| far_tests_l, local_empty_l, pair_mem_l | 1.03, 0.72, 0.68 s | 1.8% |
| ssa insert_rc (Rc.rc_func) | 40.51 s | 30.2% |

Phases (wall, loaded): check 2.71 s, ssa 1.70 s, insert_rc 10.38 s,
inline 3.89 s, inline.verify 4.25 s, emit 20.31 s, assemble 1.96 s, link
0.55 s.

What stage 4 can remove: the assembler's parse (7.48 s), the printing of
each instruction (4.27 s), and splitting and joining (about 2 s): about
14 s, 10% of this build's CPU. The line passes themselves (10.4 s, 7.8%)
move to items; whether they get cheaper there is measured with the
stage, pass by pass (copy_runs_l alone is 4.2%). Reference counting
insertion (30%) stays the largest single cost and is out of this
decision's scope.
