# 0564: One convention for wide values (by pointer across arm64, x86-64 and LLVM)

Status: Accepted (owner, 2026-11-08, on the lead's recommendation). Proposed (Xo Lang 1, 2026-11-08; owner decides)

## Context

A wide value is a struct, tuple or enum of 256 bytes or more
(BY_REF_MIN in compiler/arm64 byref.xo and compiler/llvm gen.xo). The
backends pass wide values in several ways, decided one case at a time:

| Where | arm64 and x86-64 (compiler/arm64) | LLVM (compiler/llvm) |
|---|---|---|
| parameter of a direct call | by address, struct and enum (0151, 0153); a `var` one updated in place (0148's dataflow) | by pointer, struct and enum (0145); `var` ones held in memory (0148) |
| tuple parameter | by value (words) | by value (first class aggregate) |
| parameter of an indirect call, a closure, an entry, a function whose address is taken | by value | by value |
| result | by value: result words in the caller's stack argument area | by value: a first class aggregate |
| count helpers (retain, release, drop) | no by address path found in compiler/arm64 (to confirm in step 1) | by pointer, struct, tuple and enum (rc_by_ptr) |
| equality helper | by address since 0a1b1e803 (closures.xo eq_by_ref, tuples included) | by pointer since 0a1b1e803 |
| box, optional payload, closure environment, by reference slot | words stored one at a time | one first class store; since cacc07cd0, field by field or memcpy past 4 KiB (WIDE_STORE_MIN) |
| a wide SSA value inside a function | words in virtual registers, spilled | a first class aggregate in SSA registers |

Each by value case has cost something once a program had a big enough
value:

- Equality (bench/results/wide-values-llvm-2026-11-08.md): one `==` of
  a 4,096 word struct built 801,600 bytes of native __text (423,752
  without it), took 3.77 s of native emit and 188.8 s of clang. Passing
  both values by address (0a1b1e803) made it 424,576 bytes, 0.28 s and
  15.1 s.
- Stores into memory (cacc07cd0): clang expands one first class store
  of a 4,096 word struct into a store per scalar. wbC (internal/arm64
  wide_box) went from 7.9 to 4.0 s of clang -O1 (hello world: 2.5 s),
  the golden wide_store from 18.4 to 6.8 s.
- What cacc07cd0 leaves: wbN's `main` alone still costs 1.5 s of clang
  (4.04 against 2.55 s with its body emptied), in the Machine
  Instruction Scheduler (+0.57 s), EarlyCSE (+0.45 s) and Machine Copy
  Propagation (+0.12 s). They work on non constant wide values kept as
  SSA aggregates (`b.core.s7.s7.s7.f7 = 77` is a chain of whole L3, L2
  and L1 extracts and inserts), which clang splits into thousands of
  virtual registers.
- Code size before 0145, 0151 and 0153: by value releases copied every
  word of a big struct to the stack first (xo built by LLVM: 11.15 MB of
  its 89.8, bench/results/xo-code-size-2026-10-18.md).
- Field reads of a wide value held in registers (Xo Lang 4,
  2026-11-08). On the native backend, a field read of a wide value that
  is in neither a stable memory nor a held one copies the field's words
  out of the value's word list (compiler/arm64 isel.xo field_op,
  `g.set(v, ws[off..off + n])`). In the GCP aiplatform/v1 test build
  (linux/arm64, cold, after b43fa8255), one value of the client's
  Service struct (12,040 words, about 96 KB) is kept as 12,040 virtual
  registers. Every read of its ProjectsService field copies 6,579
  words, and the generated test main alone makes 20,269 such copies,
  2.3 million words. A heap profile at the end of emit (16 MiB
  sampling) counted 796 GB allocated by the build. 307 GB of that (38%)
  is the List slices of these copies, 99.8% from field_op. The CPU
  profile puts field_op at 35 s cumulative, 10% of the build's CPU.
  Presizing the slice (l4-slice) removes only the regrowth; a home for
  the value, with field reads loading from it, removes the copy. This
  is point 4 on the native backend; Xo Lang 1 builds it with the LLVM
  side, and Xo Lang 4 reruns these profiles as its proof.

Each was a separate fix with its own threshold test (`g.size(t) >=
BY_REF_MIN`, `8 * g.nwords(t) >= BY_REF_MIN`, `WIDE_STORE_MIN`), with
tuples in some and not others. A new helper, or a new place that writes
a value to memory, starts by value again.

## Decision (proposed)

One rule, the same in compiler/arm64 (arm64 and x86-64) and
compiler/llvm: a value of a struct, tuple or enum type of BY_REF_MIN
(256) bytes or more lives in memory and moves by address.

1. **Parameters**: every call passes a wide argument by address,
   tuples, indirect calls, closures and functions whose address is taken
   included; the function type's ABI says so, so a function value and a
   direct call agree. A callee never writes through a non `var` pointer;
   a `var` one keeps 0148's in place updates. Entries (`main`, C
   exports) keep the platform ABI.
2. **Results**: a wide result is written by the callee into memory the
   caller provides (LLVM `sret`; on arm64 and x86-64 an address in a
   register, as AAPCS64's x8 and the SysV hidden pointer), not returned
   as result words.
3. **Helpers**: count, equality, hash, clone, Json and the other derive
   helpers take wide values by pointer (equality already does on every
   backend, count helpers on LLVM).
4. **Values inside a function**: a wide SSA value gets a home (a frame
   slot, or the memory of the parameter or result holding it). Field
   reads load from it, a field set stores into it in place when the old
   value is dead (0148's dataflow, from `var` parameters to every wide
   value), and a copy is a memcpy. In LLVM no first class aggregate of a
   wide type is loaded, stored, inserted into or extracted from.
5. **One predicate**: `wide(t)` in compiler/ssa, used by both backends
   and every helper generator, replacing BY_REF_MIN's three spellings and
   WIDE_STORE_MIN.

The language does not change: wide values stay values (copy semantics,
`==` by value). Only generated code changes.

## Consequences

- LLVM no longer sees wide first class aggregates: the remaining 1.5 s
  of wbN's `main` should go, and so should the cases cacc07cd0 leaves (a
  wide value returned from a call, then stored).
- Code size: by address removes the word copies at every call site, as
  0151 did for direct calls; indirect calls and closures over wide values
  gain the same.
- Indirect calls and function values change ABI (point 1). One xo
  compiles every caller and callee, so nothing is mixed; the `use c`
  boundary and C exports keep the platform ABI, through cabi's shims, as
  today.
- Wide results by `sret` (point 2) change the result words layout
  (emit.xo out_arg_bytes); the fixed point and every backend golden
  must still pass.
- A small wide value (256 bytes to about 1 KiB) can be slower by
  address than in registers in a hot loop. The plan measures that before
  each default changes; BY_REF_MIN moves in one place.
- Debug info: a wide value with a home is described by its location (a
  frame slot), which 0252's DWARF variables handle for spilled values.

## Plan (each step measured before the next)

1. The `wide(t)` predicate at today's sites, no behavior change
   (identity on every golden, arm64 and LLVM).
2. Tuples by address in direct calls and helpers (points 1 and 3 for
   tuples): identity apart from programs with wide tuples, the self
   build, `xo bench --ab` self.
3. Wide results by `sret` (point 2), both backends: the self build's
   time and __text, LLVM clang time of wide_store and wbN.
4. Wide SSA values with homes in LLVM (point 4): clang time of wbN
   (target within 0.3 s of hello world) and of xo's own LLVM build.
5. Indirect calls and closures (point 1), the ABI change, last.

## Step 4 prototype (2026-11-08, branch lang1-0564p, not pushed)

Two LLVM-only flags, both off by default and classified in
TestCodegenInputsClassified; the default build's IR is unchanged.

- `XO_WIDE_HOMES=1`: a struct, tuple or field set of 4 KiB or more
  (WIDE_STORE_MIN) is written into its own alloca (its home) where it is
  made, a field of it is a pointer into the home, and a wide variant
  keeps the memory it is built in, with no load of the whole value.
- `XO_WIDE_TAIL=1`: a wide variant zeroes only the payload past its own
  fields (an llvm.memset sized by the gap) where it stored a
  `zeroinitializer` of the whole enum before writing its fields.

clang -O1 on the module's IR (darwin/arm64, flags off then on, two
rounds each; hello world is 2.5 s):

| Program | off | XO_WIDE_TAIL | both flags |
|---|---|---|---|
| wbN (4,096 word struct of nested defaults) | 3.52 / 3.59 s | 2.36 / 3.00 s | 2.29 / 2.24 s |
| wbC (wide_box: the same with `==`) | 3.48 / 3.49 s | 2.36 / 3.47 s (noisy) | 2.28 / 2.27 s |
| golden wide_store | 6.01 / 5.86 s | 4.19 / 4.22 s | 2.33 / 2.33 s |

Both flags bring all three programs to hello world's time. Outputs are
correct with them on (wbN, wbC, wide_store on LLVM).

What did and did not matter, measured on the way:

- Homes alone first gave nothing (wbN 3.41/3.62 off against 3.61/3.87
  s on): each wide variant still loaded its whole value from the memory
  it was built in. The load was dead, but SROA splits the alloca by
  that use before anything deletes it, a load and store per word.
  Dropping it: wbN 3.56 to 3.10 s.
- The whole `zeroinitializer` store of a wide enum (32,800 bytes for
  wide_store's `Node?`) was the rest: 8 of them in main became 8,086
  `str` instructions. Zeroing only the tail: wbN 3.10 to 2.33 s.
  Replacing the store with a memset of the whole enum instead was 5x
  slower (wbN 3.6 to 17.8 s, the same final code): the memset and the
  field writes over it keep MemCpyOpt and DSE busy; the tail memset
  does not overlap the fields.
- Aliasing every narrow all-constant literal to one constant operand
  (IR 4.9 to 4.0 MB) gained nothing and changed every program's IR;
  dropped.
- Calling `memcpy` as an opaque function for the 4 KiB and larger
  copies, so clang cannot expand them, was slower (3.6 to 4.6 s);
  internal or externally initialized constant globals made no
  difference or doubled the stores.

So the cost of step 4 sits in two places only: whole loads and whole
stores of wide aggregates. The point 4 homes matter once the whole
loads go (wide_store 4.2 s with the tail alone, 2.3 s with homes).

Native: both flags are LLVM only. The native self build's __text is
38,439,548 bytes with the flags off, on, and at the base (cacc07cd0).
The self build `--ab` on linux/arm64 host 1 (base cacc07cd0, prototype
0d28dca5e, 3 rounds): wall 0.993, cpu 0.991, max RSS 1.017, on a host
loaded through all three rounds (median load 6.3 to 6.6), so within
noise, as expected of code that is byte for byte the same. That run
measured the base's xo building the tree, not each xo building itself;
the rerun with `stage2 self` (prototype 5094fd78f, 3 rounds, 2 loaded)
gives wall 1.002 (0.997 to 1.008, within noise), cpu 1.002, max RSS
1.008.

xo's own LLVM IR does change with XO_WIDE_HOMES (376.8 to 377.6 MB):
xo has types of 4 KiB and more (arm64.G at 5,696 bytes, check.Checker,
and others of 4,688 to 8,600 bytes), copied by memcpy since ed685dd2f.
The clang time of xo's LLVM build with the flags is the figure step 4
still owes.

## Native field views (2026-11-08, behind XO_FIELD_VIEW=1, default off)

A fresh profile at main (fbc2c5c19, a stage 2 xo, linux/arm64 host 1,
the aiplatform/v1 test build: wall 1:07, emit 44.5 s) put 127.4 GB of
the build's 497 GB allocated, and 49 s (13.3%) of its CPU, in one
slice: compiler/arm64 isel.xo field_op's `ws[off..off + n]`, which
copies a field's words out of its aggregate's word list (99.7% of
xo.slice[List[Int]]). That is point 4's native half for values in
registers.

`XO_FIELD_VIEW=1` keeps a field of 8 words or more (FIELD_VIEW_MIN)
that is not a bound value as a view, (root value, word offset) in
G.fview, and slices the root's words only where a use needs them
(words()); a view whose root is in registers keeps that slice for its
next use. The flag is keyed in the function and part caches (` fview=1`)
and classified in TestCodegenInputsClassified.

Measured on host 1 (stage 2 xo of 3642d3ad7, before the slice is kept
for later uses; one run each):

| aiplatform/v1 test build | off | XO_FIELD_VIEW=1 |
|---|---|---|
| xo.slice[List[Int]] allocated | 126.2 GB (25.8%) | 0.5 GB (0.14%) |
| its CPU | 51.7 s (13.9%) | 0.1 s |
| CPU samples, whole build | 37,355 | 26,962 |
| emit phase | 45.0 s | 35.7 s |
| heap at emit (peak) | 6,748 MB (6,874) | 4,668 MB (4,794) |
| max RSS | 6.94 GB | 6.26 GB |
| wall | 1:08.0 | 0:58.2 |

Output: scripts/identity of the xo at f3aa48965 against itself with the
flag on (host 2, linux/arm64, every second golden and compiler/xo): 322
programs, 0 differ. Codegen patterns pass with the flag off and on. The
self build `--ab` (stage2 self, the flag forced on in a throwaway
commit) was 1.002 wall but on a loaded host (4 of 5 rounds), so it is
rerun unloaded before the default changes.

## Step 1 (2026-11-08): one predicate

compiler/ssa wide.xo holds the threshold (WIDE_MIN, 256 bytes) and the
predicates (wide_kind: struct, tuple or enum; wide_bytes), in place of
compiler/llvm gen.xo's and compiler/arm64 byref.xo's BY_REF_MIN and
compiler/ssa chainpads.xo's HOME_MIN_BYTES. Each site keeps the kinds it
had (tuples where it took them) and each backend its measure: LLVM its
layout's bytes, the native backends 8 bytes a word, so a type of many
narrow fields can be wide natively and not on LLVM until a later step
picks one measure. WIDE_STORE_MIN stays LLVM's store threshold (how a
value is written to memory, not whether it is wide).

Confirmed while doing it: compiler/arm64's count helpers take a wide
struct's words by value (calls.xo retain and release call rc_helper's
dup and drop with every word, unless the type has one heap leaf); point
3 changes that.

No output change: scripts/identity against its parent, linux/arm64,
every second golden and compiler/xo, 322 programs, 0 differ (arm64).
The LLVM identity run (every eighth golden) ran past its 90 minute
limit on host 2; each LLVM site's change is the same comparison spelled
through wide_bytes, and the LLVM goldens of step 2's gate pass.

## Step 2 (2026-11-08): wide tuples by address in direct calls

A tuple of ssa.WIDE_MIN bytes or more is now a by address parameter of a
direct call on every native backend and LLVM, as structs (0145, 0151) and
enums (0153) were: compiler/arm64 byref.xo big_by_ref and compiler/llvm
gen.xo by_ref_of take wide_kind, and compiler/ssa chainpads' home types
too. On the native backends a wide tuple field or payload is then also
read in place where its parent is in memory, as a struct's is.

One exception (byref.xo frame_homes_of): a `var` argument that is a field
of a wide tuple does not make the tuple's type a frame home. The results
of a call with `var` parameters are one tuple (the result and each `var`
parameter's new value), and taking its type as a home passed the field
at that tuple's home: byref_held, byref_home_loop, byref_struct and
byref_tuple segfaulted (exit 139) on arm64 until that site skipped tuple
parents. Open item: whether a wide tuple that is not a result tuple
should get a home there (it does not now).

Proof (the calling convention changes, so identity against step 1
differs by design): golden byref_tuple (read only and `var` tuple
parameters, two in one call, recursion, a loop, a wide tuple in a wide
tuple and in a struct, a function value); the byref, tuple and wide
goldens on arm64 and LLVM (15 of 15 each); codegen patterns (42 of 42);
self build; fixed point; internal/arm64's full tests on host 2
(linux/arm64, 1,134 s, pass); internal/llvm with the goldens
the change touches and the -O2 sample. Against step 1 (scripts/identity, linux/arm64, every second golden
and compiler/xo, 322 programs) only byref_tuple's output changes: xo
itself passes no wide tuple.

## Step 3 (2026-11-09): wide results through the caller's memory, held

Branches lang1-0564s3 (native) and lang1-0564s3l (LLVM), not pushed. A
wide declared result of a function called directly (not indirect, not an
entry, not address taken, not main) is returned through memory: the
caller passes a slot's address after the arguments, the callee stores
the result there (native: an extra entry def; LLVM: a trailing
`ptr noalias %xo.sret` parameter, `void` when nothing else is returned).
The caller reads a result in place (its slot planned per call by block
liveness, shared between calls whose results do not overlap) or loads
it at once from one slot per type. xo's own build has 226 functions with
wide declared results.

Measured (2026-11-09): all 644 goldens pass on arm64, also with the
self-built xo; the 16 wide goldens on llvm and on darwin/amd64. xo's
__text 0.94% smaller; bench/perf/x509 parse --ab 0.992 (host 1,
unloaded); the self build within noise (loaded rounds). But xo's stack
frames grow 11% in total, the parser's recursion most: parse_stmt from
2,288 to 8,400 bytes. Before step 3 a wide result came back in the
outgoing area past the registers, and the register allocator's
arg_slots pass used those words as their spill homes at no frame cost;
in a slot of the caller's frame its words take ordinary spill slots
(parse_stmt 12 to about 170). The code is smaller and does less stack
traffic (parse_stmt's stores to sp 1,788 to 505), so the cost is
recursion depth, which matters on ThreadSanitizer's 2 MB fiber stacks.

Owner decision (2026-11-09): step 3 is held until the spill slot work
restores those homes; it lands when frames are back to their size
before step 3 (parse_stmt about 2.3 KB) with the gain kept. Next, in
order: find which wide value is live across a call as words (parse_stmt
has about one 171 word Stmt spilled); give words loaded from a result
slot that slot as their spill home (an arg_slots like hint, ended by the
next call writing the slot); store a constructed wide value's fields
straight to the result slot.

## Step 3 (2026-11-10): native sret for wide results

A wide result (big_by_ref) of a direct call is written by the callee
through an address the caller passes after the arguments, as one more
integer word (byref.xo sret_of; not for indirect, entry or address-taken
functions), so the caller no longer moves it word by word. Where the caller keeps it:

- **forwarded**: a call whose result is the caller's own return value gets
  the caller's sret address (plan_sret_forward): nothing is copied;
- **in place** in a frame slot when every use reads it by parts (Field,
  Payload, Tag, and a Release of a struct or tuple, which drops its
  leaves from memory; plan_sret_slots), slots shared by calls whose
  results are not live at once; not for a call with `var` results, whose
  tuple holds the receiver;
- **the outgoing area** past the call's stack arguments (sret_slot): up to
  700 words, or 256 when that pushes the frame past the ldr reach (the
  function is lowered again), loaded where used, the words keeping
  outgoing slots as spill homes until the next call (as results past the
  registers did before);
- **at a Return**: a result held in memory (a by-reference parameter, a
  slot) is copied to the sret address with memcpy (ret_sret), and a struct
  or tuple made only to be returned is written there field by field
  (sret_mk), so neither is loaded whole and spilled.

Measured on xo's own build (arm64, compiler/xo): total frame size
7,160,336 to 7,171,520 bytes (+0.16%), __text 40,351,028 to 40,302,836
(-0.12%). The first form (every wide result in its own frame slot) was
-0.94% __text but +11% frames (parse_stmt 2,288 to 9,136 bytes); the
variants between are in the lead's notes (0564-frontier-2026-11-10.md):

| Variant | total frames | vs base | __text | vs base |
|---|---|---|---|---|
| first form (2026-11-09) | about 7.9 MB | +11% | | -0.94% |
| k3: in place, Return copies from memory | 7,241,408 | +1.14% | 40,173,684 | -0.44% |
| kh (shipped): k3 plus the 700-word outgoing area | 7,171,520 | +0.16% | 40,302,836 | -0.12% |

The owner held step 3 (2026-11-09) until frames were back to their size
with the gain kept, and accepted kh as meeting that bar (2026-11-10,
relayed by the lead). A later step recovers more of the __text gain
without growing frames.
Census findings that shaped it: of 5,724 results rejected for in-place
reading, 5,589 were rejected only by a Release of the whole value; a
`return (c, ...)` of a by-reference parameter loaded its 608 words and
spilled them to the frame (fuzz_check, 10,176 to 5,328 bytes).

## Alternatives

- **Keep fixing case by case** (today). It works, but each new helper
  or memory write starts by value; the equality case cost a 188.8 s
  clang build before anyone looked.
- **A higher threshold** (4 KiB, as WIDE_STORE_MIN): fewer programs
  change, but copying 256 byte to 4 KiB values at calls (the 11 MB of
  2026-10-18) comes back.
- **Leave LLVM's aggregates and run SROA earlier**: SROA is what splits
  them; the split itself is the cost.
- **Box wide values on the heap** (as recursive types are): an
  allocation per value, and copy semantics then need copy on write; far
  more run time than a frame home.
