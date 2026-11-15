# Local-first UI experiments (decision 0659), 2026-11-15

Two experiments behind decision 0659. The code is not on main: it is on
the local branch `ui-recon` (worktree ~/xo-wt/ui-recon, commits
e2081d074 and 8a0e27be3), experiments/ui-recon (recon.xo, recon_test.xo)
and experiments/ui-layout (layout.xo, tasks/, grader/); the agent
workspaces and graders are also in ~/xo-lead/notes/ui-layout. Run on the
Mac with the xo built from b4026d8ef.

## 1. Local-first reconciliation: native experiment (2026-11-15)

Question: can one pure `update(model, msg)`, run on clients (predicted,
rebased) and on the server (authoritative, in arrival order), keep every
replica correct over a bad network, so a UI needs one mode instead of
static/live/islands/wasm?

Setup (recon.xo): shared todo list (add, rename, toggle, star, remove) plus
a 5 seat `Reserve` that needs the server's answer. Clients apply Msgs at
once and queue them; the server applies each (client, seq) exactly once and
appends to a log; clients replay the log with the same update and fold
pending Msgs on top. The network drops, delays (reorders) and duplicates
packets; clients go offline for stretches and keep working. 200 seeds per
profile; after the run the network heals and runs until quiet.

Checks: at every confirmed log version a client's model equals the
server's at that version ("divergences"); at the end every view equals the
server's model with nothing pending ("converged"); the log holds each
issued Msg exactly once and replays to the server's model.

| profile | drop | offline% of actions | converged | divergences | overturned predictions | Up resend x |
|---|---|---|---|---|---|---|
| lan+ack | 0% | 0% | 200/200 | 0 | 718 of 47898 (1.5%) | 1.0 |
| mobile+ack | 10% | 22% | 200/200 | 0 | 4440 of 72362 (6.1%) | 3.7 |
| subway+ack | 30% | 67% | 200/200 | 0 | 13757 of 95671 (14.4%) | 7.8 |
| hostile+ack | 60%, 20% dup | 83% | 200/200 | 0 | 2289 of 143833 (1.6%) | 8.6 |
| drift (control) | 10% | 22% | 0/200 | 70273 | | |

Without server acks the client resends every unconfirmed Msg on each sync:
52.5x (subway) and 276.9x (hostile). Adding `acked` (last applied seq) to
the server's reply cut that to 7.8x and 8.6x; correctness was the same.

The drift control (client 0's Star adds 2, as a stale bundle or an impure
read would) is caught at the first confirmed Star (seed 1, version 20)
and never converges, so the checks are not vacuous.

Property tests (recon_test.xo, `xo test .`): every base profile with acks
converges for any seed; the drifted client is detected. 2 passed.

### What this shows

- Correctness holds: the model never diverged, offline work of up to 83%
  of all actions merged, delivery was exactly once under loss, duplication
  and reordering. The precondition is that update is deterministic and
  identical on both sides; the control shows what breaks otherwise, so
  the compiler must enforce it (no clock, rand, I/O in update; one
  compiled update shipped to both sides).
- The real UX cost is overturned predictions: 1.5% to 14% of a client's
  own actions were undone by the server (an edit to a todo someone else
  removed, a seat taken while offline). A UI framework must make those
  visible (Msgs typed as needing confirmation, a pending state in the
  view), not silently roll back.
- The wire protocol needs acks and batching; both are small.

### Not covered yet

Full replication only (no partial subscriptions or server-only fields);
pending Msgs are not persisted across a reload; no authorization per Msg;
no browser, no bundle size; one server process (no multi-node log).

## 2. Layout primitives and text wireframes: first agent experiment (2026-11-15)

Question: does a constrained layout vocabulary with a compiler-computed
text wireframe (and geometric diagnostics) let an agent fix visual bugs
better than HTML/CSS with screenshots?

Setup: 10 pages, one planted bug each (overflow, truncation, tap target,
contrast, misaligned form, wrong button order, squeezed sidebar, uneven
spacing, fixed grid, unaligned amounts), written twice: HTML/CSS and the
.ui vocabulary (layout.xo: parser, layout engine, wireframe, XU01-XU05).
Three fresh agents (same model), same INTENT.md per task:
A = HTML + screenshots at 390/760/1280 (headless Chrome, exact-width
iframe), B = .ui + wireframe, C = .ui + wireframe + diagnostics.
Hidden graders: Chrome JS checks (scroll width, ellipsis, button width,
WCAG contrast, page-specific geometry) for A; `layout diag` plus
`layout assert` (right_of, equal_gaps) for B and C; visible text
unchanged for all. Graders validated: all 10 broken pages fail, all 10
reference fixes pass, in both forms.

| condition | pass | render calls | agent tokens | tool uses | wall time |
|---|---|---|---|---|---|
| A: HTML + screenshots | 10/10 | 11 | 76,326 | 21 | 86 s |
| B: .ui + wireframe | 10/10 | 17 | 70,978 | 15 | 92 s |
| C: .ui + wireframe + diagnostics | 10/10 | 28 | 74,110 | 22 | 108 s |

(B and C first scored 9/10: the grader compared `field "Name"` with
`text "Name"` as different text; the reference fix failed the same way.
Fixed in the grader; no agent output changed.)

### Reading

- No difference: a ceiling effect. Every agent fixed every bug. Each page
  had one planted bug, the requirement list named its category, and the
  pages were 6 to 25 lines, so the bug was findable by reading the
  source. Agent A took 1 screenshot round per task (2 for 09) and fixed
  mostly from the CSS.
- The new vocabulary cost nothing to learn: B and C read a 42 to 53 line
  VOCAB.md and passed 10/10 with idiomatic fixes (sidebar, form, grid
  min, wrap) and similar token use to A.
- Diagnostics did not reduce iterations here; C rendered more often (it
  re-ran to confirm "no diagnostics").
- This experiment does not show the wireframe beats screenshots. It
  shows parity on easy, single-bug pages, with a vocabulary learned from
  one page.

### Next experiment (harder, to break the ceiling)

1. Unlabeled, multi-bug pages: 60 to 150 lines, 2 to 4 bugs each, no
   requirement checklist naming categories ("users say it looks off").
2. Bugs that only appear from geometry, not source: lengths that fit at
   760 but not 390, contrast via inherited tone, alignment broken by one
   long label.
3. Authoring from a description (build the page, graded by the same
   checks), where the screenshot loop is the only feedback in HTML.
4. Score regressions: a fix that breaks another width counts as a fail.
