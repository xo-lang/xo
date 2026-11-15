# 0653: new features ship a vet rule and fix; adoption runs in sweeps at seed cuts

Status: Accepted (owner, 2026-11-15). Drafted by the lead; built and run
by Xo Lang 2.

## Context

On 2026-11-15 the owner asked whether compiler and runtime code uses the
language features already shipped. Lang 1's `xo vet` audit (main
c69164409, ~/xo-lead/notes/lang1/vet) says mostly not:

- XO0906 (the `??` form): compiler 319, std 53, std/native 103, llrt 120.
- A destructure then assign that one 0536 assignment writes: compiler
  213, std 19, std/native 42, llrt 59.
- About 600 more vet sites (XO0208, XO0903, XO0530, XO0703, XO0532) and
  353 `vet --cost` notes (XO0907 to XO0914), most in the compiler.

Most of it is readability; a few hot sites are also faster (template
render, jwt.xo:179, netip). Two constraints shape when the conversion
can happen:

- The seed rule: code the seed compiles (compiler/, and the std/native
  and llrt files xo imports) uses a new form only after a seed that
  parses it is cut.
- Converting at each feature's landing puts a large mechanical diff
  beside every feature push, conflicts with every worker's branch, and
  multiplies gates. Most vet codes today carry no machine fix, so each
  conversion is by hand.

## Decision

1. **Every feature ADR that makes an older form obsolete ships, in the
   same push, a vet code that finds the old form and an exact machine
   fix** that `xo fix` and `xo check --fix` (decision 0096) apply. A
   feature with no older form to replace needs neither. The ADR names
   the code, or says why there is none.
2. **The feature's push converts nothing.** It carries the feature, its
   tests, and the vet rule with its fix tests.
3. **Adoption runs in sweeps, one per seed cut** (about daily), owned by
   one worker (Lang 2, who cuts the seeds):
   - For every feature the new seed parses, run `xo fix` over compiler/,
     std/native, llrt and the linkers.
   - Code the seed does not compile (std outside native, tools, tests,
     bench) takes a feature at the next sweep after it lands, without
     waiting for a seed.
   - One push per directory or area (for example compiler/arm64, then
     compiler/ssa), purely mechanical, no other edits.
4. **Gate by kind of feature:**
   - Readability forms: the sweep claims no output change and its gate
     runs the identity check (byte-identical xo built by the old and new
     trees), plus the self build when it touches what xo builds from. No
     `--ab`.
   - Performance forms: the output changes on purpose. The gate runs the
     self build and one `--ab` per sweep on the linux/arm64 offload host
     (xo building itself, and the std benches the vet notes point at).
     A directory that measures slower is dropped from the sweep and
     reported, not pushed.
5. **Regenerate, do not merge.** A sweep that conflicts with main or
   with an in-flight branch is redone with `xo fix` on the new main, not
   resolved by hand. Sweeps push when the push queue is short.
6. **Ratchet.** Once a directory is converted for a code, the sweep adds
   it to a list that its vet check holds at zero, so the old form does
   not come back. A false positive or a site where the new form reads
   worse is recorded and the vet rule is tuned, not suppressed site by
   site.
7. **Priority.** Std Proven work (Lang 1's perf fixes A to E, the
   evidence pass) comes first when the two compete for host 1. The hot
   sites Lang 1 found (template, jwt, netip) stay with Lang 1 as perf
   work.

## First steps

1. Machine fixes for XO0906 (`??`, about 600 sites) and the
   destructure then assign form (about 330 sites; give it a vet code
   if it has none).
2. The ratchet list and the sweep script (scripts/, one command per
   sweep: fix, format, per-directory commits, the gate per kind).
3. The backlog as the first sweeps, after Lang 1's fix A lands.
4. 0649, 0650 and 0651 get their vet rules (or a note that none applies)
   as they are built; they enter a sweep at the seed that parses them.

## Consequences

- Feature pushes stay small; conversion load comes once a day, from one
  owner, with tool generated diffs that are cheap to regenerate.
- The compiler and runtime pick up performance features within about a
  day of the seed that parses them.
- Vet rules gain machine fixes, which also helps agents writing new code
  (decision 0096's loop).
- Feature authors carry the cost of writing the fix.

## Alternatives

- Convert at each feature's landing: rejected (large diffs beside every
  feature, constant conflicts, more gates).
- A fixed daily clock unrelated to seeds: rejected; seed compiled code
  cannot adopt before the seed, so the seed cut is the natural point.
- No conversion, only new code uses new forms: rejected; the
  performance forms leave speed unused in the compiler and runtime.
