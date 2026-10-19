# Draft: How agents build Xo with Xo's own tools

Status: Draft (Xo Lang 3, 2026-10-19, for the owner and Xo Publish; not
published). Every figure cites the committed file and line it comes
from; most are in bench/results, and the few from infrastructure docs
are marked as such.

---

Xo is written by AI agents, in Xo, with tools that are themselves
written in Xo. Several agents work in parallel on one Mac, each in its
own git worktree, on the compiler, the runtime, the standard library,
and the benchmarks. For the last week, a large share of their work has
gone into the loop around the code: how fast an agent finds what it
needs, sees what an edit did, and proves that the change is safe to
land. This post covers what that loop is made of, what each part
changed, and where we got it wrong.

## Finding code: ask the compiler, not grep

`xo def`, `xo refs`, `xo type FILE:LINE:COL`, and `xo doc` (decision
0261) answer the questions an agent otherwise answers with grep and a
guess: where a name is declared, who calls it, what type an expression
has, and which methods a type has. The guess was the expensive part.
Agents wrote calls to methods that did not exist while the real one sat
in the prelude: `Map.update` was there while agents reached for
`update_or` (knowledgebase/infrastructure/agent-tools.md:12-13), and
the same method was behind one of the habit failures in the early
blind-written benchmark programs
(bench/results/habit-diagnostics-2026-10-09.md:27).

## The edit loop: from a minute to six seconds

The compiler is about 171k lines of Xo
(bench/results/xo-code-size-2026-10-18.md:4), and every compiler change
rebuilds it. So the time to rebuild xo after a one-line edit is the
agents' inner loop. The build caches its work at each stage, keyed on
content, and each stage was measured on the same edit (one constant in
the compiler's fix module), wall clock on the development Mac:

| Stage | Rebuild of xo after the edit | Source |
|---|---|---|
| no cache | 60.15 s | bench/results/fast-builds-2026-10-13.md:439 |
| A: an object per part | 22.72 s | fast-builds-2026-10-13.md:442 |
| C: release insertion per function | 11.28 s | fast-builds-2026-10-13.md:542 |
| D 2f: lazy decoding, packs, parts, parallel borrow rounds | 6.20 to 6.45 s | fast-builds-2026-10-13.md:783-785 |

The cache is checked, not trusted: a cache-off build must produce the
same file byte for byte, and each stage's measurement says so (for 2f,
fast-builds-2026-10-13.md:790-791). What is left of the six seconds is
listed beside the table (fast-builds-2026-10-13.md:793-795): the SSA
builder about 2.0 s, the checker 1.0 s, inlining 0.6 to 0.7 s.

The test suite got the same treatment. The repository's `-short` run,
the per commit check, went from 35.0 s to 15.7 s, and `./cmd/xo -short`
from 24.6 s to 5.7 s (bench/results/short-tests-2026-10-14.md:21-22).
The LLVM backend's full tests spent most of their time in clang on
unchanged IR. A caching clang keyed on the IR's contents cut the
package from 3,333 s to 1,692 s cold and 567 s warm (infrastructure
doc: knowledgebase/infrastructure/toolchain.md:1505-1506).

## Seeing what an edit did

Performance work runs against instruction counts, not wall time. On a
shared machine, wall time moves by up to 2x between identical builds
(fast-builds-2026-10-13.md:432-434). The HTTP benchmark runs the server
under callgrind in a Linux container and reports instructions per
request for each scenario. `xo dev compare BASE NEW` (decision 0264)
puts two such runs side by side and lists the functions whose costs
changed most. `--callers=REGEX` answers "who allocates per request",
with names as Xo writes them, not as the linker mangles them
(agent-tools.md:83-87).

The useful habit is to read the function list, not just the total.
Today an out-of-line release path for the LLVM backend made the
compiler 37% smaller but HTTP about 1.5% slower. The function list
showed why at once: string literals have an immortal reference count,
and every copy and release of one now took the out-of-line call
(xo-code-size-2026-10-18.md:479-484). Keeping that one check inline
gave a version that was both smaller and faster.

The same habit catches measurements too noisy to judge. A short
callgrind run of an LLVM build spreads about 3% from run to run, so it
cannot confirm a 1% change; the full mode, with more requests and
repeats, stays within about 0.2% (xo-code-size-2026-10-18.md:486-488,
507-509).

## Size as a column, not an afterthought

`scripts/codesize.py` breaks a binary's code down by function, module,
generic copy, and instruction kind. It showed that xo's size came from
how the native backend copies and releases large values, not from
generics or inlining (xo-code-size-2026-10-18.md:12-20). Since
2026-10-19, a code generation change reports an LLVM column next to the
native one (xo-code-size-2026-10-18.md:409). The LLVM-built xo went
from 89.82 MB of code to 50.56 MB (xo-code-size-2026-10-18.md:404),
then to 32.59 MB (line 473), close to the native backend's 31.78 MB
(line 362). The Windows release binary built through LLVM went from
80.56 MB to 63.65 MB (line 500).

## Landing a change

Each agent runs its own gate, `scripts/gate.sh WORKTREE STEP...`, with
named steps: the short tests, a backend's full tests, the generator
goldens, and the self rebuild fixed point. The fixed point means xo
builds itself twice and both builds must be byte-identical, so a
miscompile cannot build the next compiler. The LLVM step skips itself
when a change touches only the native backend (scripts/gate.sh:12,
83). Heavy runs take a slot from a machine-wide lock, at most one per
agent. A push goes through a queue that rebases onto main and re-runs
the gate only when the change overlaps something that landed in the
meantime. A shared board shows what each agent is doing and what it
will do next.

When a program misbehaves, `xo dev bisect-pass DIR` turns the
optimizer's passes on one prefix at a time and names the first pass
that changes the output, and a reducer shrinks the failing program
(agent-tools.md:27-31).

## What we got wrong

- **LLVM was not measured.** For most of the project, size and speed
  tables had only the native backend. The LLVM-built compiler was 2.83x
  the native one's size (xo-code-size-2026-10-18.md:413) before anyone
  looked, and two causes alone (releases on rarely run paths, and each
  inlined release's own slow path) were about 12 MB
  (xo-code-size-2026-10-18.md:452). The LLVM column is
  now required.
- **Capacity before profiling.** A burst of 2,000 connections in the
  Linux container sometimes took over a second. The first suspect was
  the runtime's fiber stack cache, which is too small for such a burst
  (it maps and unmaps about 1,840 stacks a round,
  knowledgebase/gotchas/docker-burst-stalls-on-loaded-mac.md:37-41).
  Profiling showed the real cause: the whole Linux VM was descheduled
  by macOS under the other agents' load, and with client and server on
  separate CPUs the Xo server matched Go's
  (docker-burst-stalls-on-loaded-mac.md:15-28). The cache is still
  worth fixing, but on its own evidence.
- **Measuring with too blunt a tool.** A 3% run-to-run spread passed
  for a signal more than once before the full callgrind mode became the
  rule for LLVM speed checks (xo-code-size-2026-10-18.md:507-509).

## What is next

The six seconds left in the edit loop are mostly in the front end
(checking and SSA building), which the caches do not cover yet. Every
size and speed table now carries both backends.
