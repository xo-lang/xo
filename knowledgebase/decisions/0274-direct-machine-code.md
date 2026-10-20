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
