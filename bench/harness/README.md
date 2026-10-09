# Agent benchmark harness (any model)

Reruns the Xo agent benchmarks (stage 3 three arm comparison and the planted
bug benchmark) with a model other than Claude, outside Claude Code, and
scores the results the same way as the published runs
(`bench/results/three-arm-2026-10-08.md`, `planted-and-harder-2026-10-07.md`,
`planted-xo-2026-10-07.md`). Python 3 standard library only, plus the Go
toolchain and the strict analyzers already used by the benchmark.

| File | What it is |
|---|---|
| `prompts/<arm>/<task>.md` | the prompt for one task in one arm (`xo`, `go-plain`, `go-strict`); includes are expanded by `prompts.py` |
| `prompts/planted/<set>/<task>.md` | the planted bug author prompts (`go`, `xo-translate`, `xo-independent`), tasks 01 to 05 |
| `prompts/common/` | shared pieces: Xo spec context, the three run protocols, output contracts, bug classes |
| `prompts.py` | renders prompts (`render`, `all`) and estimates token cost (`tokens`) |
| `run_openai_compatible.py` | optional adapter: sends the prompts to any OpenAI compatible chat endpoint and runs the feedback loop |
| `score.py` | scores a directory of model outputs (`solutions`, `planted`); `--claude` scores the existing Claude outputs |
| `hidden/crud_api.py` | hidden black box HTTP checks for task 01 |
| `test_harness.py` | self test (fake local endpoint, no network); also run by `go test ./internal/benchharness` |

## Step by step

### 1. Prerequisites (once)

From the repo root:

```
go version                       # 1.26.x
ls .tools/bin                    # staticcheck errcheck nilaway exhaustive (see knowledgebase/infrastructure/toolchain.md)
python3 bench/harness/test_harness.py
```

The scorer builds `xo` from this checkout (the current toolchain) into
`$TMPDIR/xo-harness/xo`. Set `XO_BIN=/path/to/xo` to score with another
build (for example the frozen `.tools/bin/xo-0.6`), and `XO_TOOLS_BIN` when
the analyzers are not in `.tools/bin` (in a git worktree the scorer also
looks in the main checkout's `.tools/bin`).

### 2a. Run a model with the adapter

```
export OPENAI_BASE_URL=https://api.openai.com/v1           # or any OpenAI compatible endpoint
export OPENAI_API_KEY=...                                  # never written to disk by the harness
export OPENAI_MODEL=gpt-5
# optional: export OPENAI_EXTRA_BODY='{"reasoning_effort": "high"}'

python3 bench/harness/run_openai_compatible.py --out runs/gpt-5 --tasks 01,09 --planted none   # small sample first
python3 bench/harness/run_openai_compatible.py --out runs/gpt-5 --resume                        # then everything
```

Gemini: `OPENAI_BASE_URL=https://generativelanguage.googleapis.com/v1beta/openai`,
`OPENAI_MODEL=gemini-2.5-pro`, and a Gemini API key. Local servers (vLLM,
llama.cpp, Ollama) work the same way. `--resume` skips cells already done,
so an interrupted run can continue. `--dry-run` only writes the rendered
prompts.

### 2b. Or run the model any other way

Render the prompts and give each one to the model (web chat, another agent
framework, a batch API):

```
python3 bench/harness/prompts.py all --out runs/mymodel/prompts
python3 bench/harness/prompts.py render xo 09          # one prompt on stdout
```

Put each answer's files in `runs/mymodel/<arm>/<task>/` (solutions) or
`runs/mymodel/planted/<set>/<task>/` (planted bugs: `manifest.json` plus
`variants/<Id>/...`). To record the iteration history yourself, write one
JSON line per run to `<arm>/<task>/runs.jsonl`, for example
`{"run": 1, "green": false, "stage": "test"}`. Without it the scorer can only
report the final state, not runs to green.

### 3. Score

```
python3 bench/harness/score.py solutions runs/gpt-5 --out runs/gpt-5/solutions.md --json runs/gpt-5/solutions.jsonl
python3 bench/harness/score.py planted runs/gpt-5 --out runs/gpt-5/planted.md --json runs/gpt-5/planted.jsonl
```

Both take `--tasks 01,09`, and `--arms` or `--sets`. Each cell is copied to
a temporary directory and rerun, so the scorer never changes the outputs.
The scorer lowers its own priority (`--nice 10`); runs are sequential.
Expect about 15 minutes for all solutions and about 60 minutes for all
planted sets (the race detector and nilaway dominate).

### 4. Compare against the published Claude results

```
python3 bench/harness/score.py solutions --claude --out /tmp/claude-solutions.md
python3 bench/harness/score.py planted --claude --out /tmp/claude-planted.md
```

Put the two reports side by side. The solutions report has the same table
as `three-arm-2026-10-08.md` (runs to green per arm, Xo tests when first run,
lines, test counts) plus a summary (first try pass rate, total runs, lines,
green now, diagnostics on the final code, hidden checks). The planted
report has the class tables of `planted-and-harder-2026-10-07.md` (Go) and
`planted-xo-2026-10-07.md` (Xo). Write the comparison as a new
`bench/results/<model>-YYYY-MM-DD.md` and link it from `bench/README.md`.

## What the scorer measures

| Column | How |
|---|---|
| Runs to green | first green run in `runs.jsonl`, else in the cell's `BUILD_LOG.md` table (the original runs) |
| First try pass rate | run 1 green, per arm. Xo runs since 2026-10-09 get `xo check --fix` in their loop (decision 0096), so this is after fix; the as written rate is "First try now". `runs.jsonl` records `fixes` per Xo run |
| First try now | the first run's sources (`<cell>/.attempts/run-1` from the adapter, or `<out>/history/<task>/1` for a path 2b run) rerun through the arm's loop with the current toolchain |
| First try after `xo check --fix` | Xo only: the same sources through `xo check --fix` (the machine fixes of the diagnostics, decision 0096), then test; the per cell table "First runs" shows the codes left and the number of fixes. "First try now" stays as written (read only `xo check`) |
| Xo tests when first run | from `runs.jsonl`; for the original runs, the rerun of the final code (their tests had never run before) |
| Lines, tests | `.go` or `.xo` lines including tests; `func Test` in `_test.go`, `test "..."` blocks in `_test.xo` |
| Green now | the final code through its arm's loop with the current toolchain |
| Diagnostics | Xo: `xo check` errors plus warnings. Go (both arms): strict analyzer findings, one per reported position plus each data race; for plain Go this is the "escaped findings" measure |
| Hidden checks | task 01 only: build the root, serve on `PORT`, 17 black box HTTP checks of behavior the task states |
| Planted, Go | caught by the plain loop (build, vet, a test failing that passes on the base) or by a strict tool finding the base does not have |
| Planted, Xo | inexpressible (manifest), caught by `xo check` (any error), caught by `xo test --race` (only tests that pass on the unplanted base count; names compared without their `(file:line, seed)` suffix; since 2026-10-09), escaped with a new warning, escaped |

## Protocol and differences from the original runs

The original subjects were Claude agents in Claude Code. Their prompts
were not saved; the prompts here are reconstructed from what the results
and logs record (bench/README.md stage 3, the `BUILD_LOG.md` and `NOTES.md`
conventions, `bench/planted/README.md`, and the result files). Differences:

1. **Tool access.** Original subjects ran the commands themselves and could
   read any file; here the harness runs the arm's loop after every reply and
   sends the output back (the first 6000 and last 6000 characters when
   longer). Same rule: the whole solution is written before any command, and
   the run stops at green. Extra probe runs (the Xo v0.4 subjects planted
   deliberate errors to see the checker work) are not possible and were not
   counted originally either.
2. **Run cap.** The originals had no cap (at most 2 runs to green were
   needed). The adapter stops at `--max-runs` (default 5); a cell not green
   by then shows "not green in 5".
3. **Xo loop.** Tasks 01 to 08 were originally written against the checker
   only (`xo-0.4 check`, no test runner existed); tasks 09 and 10 used
   `xo check` then `xo test --race` (spec 0.6). The harness uses check then
   test for every task with the current spec and toolchain, so a new Xo run
   is stricter than the 01 to 08 originals.
4. **Context.** The Xo prompt includes all of `spec/core.md` and
   `spec/std.md` (about 31k tokens); originals read the same files from the
   repo. The Go prompts add one paragraph translating the task's neutral
   signature notation (`List[Job]`, `T ! E`), which the originals had to
   infer.
5. **Logs.** Original subjects wrote `BUILD_LOG.md`; here the harness writes
   `runs.jsonl` and `transcript.jsonl`, and the model writes only `NOTES.md`.
6. **Planted authors.** Same classes, the same "realism, no checks" rule, and
   the same bases (plain Go solutions; Xo v0.4 solutions). `xo-translate`
   matches author 1 (translating the Go manifest, X and N classes; the
   original used two agents for X and N), `xo-independent` matches author 2
   (X only, "prefer subtle, deliberate looking forms", no Go manifest). One
   extra request is allowed only when a reply has no valid `manifest.json`.
7. **Hidden checks** did not exist in the original runs; they are a separate
   column and do not change runs to green.

## Validation against the existing Claude outputs (2026-10-08)

Scoring the stored Claude outputs reproduces the published numbers
(`bench/results/harness-validation-2026-10-08.md`): every runs to green,
line, and test count of the three arm table (10 of 10 tasks); escaped
strict findings on plain Go; every Go planted variant's plain and strict
result (9/30 and 14/30 raw, 13/30 after the published manual exclusion);
Xo author 1 at 29/30 and N 16/20; Xo author 2 at 23/30 (25 with warnings).
With today's toolchain two Xo cells are no longer green: 03 fails its
contract fuzz (the known `backoff` overflow) and 06 fails XO0410, a spec 0.6
rule.

## Cost estimate

`python3 bench/harness/prompts.py tokens` prints the estimate per prompt and
per full run (4 characters per token; reply sizes taken from the Claude
outputs; each retry resends the conversation). At 2026-10-08:

| Part | Prompts | Input tokens, Claude's runs per cell | Output tokens |
|---|---|---|---|
| Xo solutions | 10 | 363k | 62k |
| Go plain solutions | 10 | 30k | 77k |
| Go strict solutions | 10 | 85k | 101k |
| Planted authors (go, xo-translate, xo-independent) | 15 | 378k | 159k |
| **Full run** | **45** | **about 0.86M** | **about 0.40M** |

Bounds: every cell green on run 1 is 0.72M in and 0.36M out; every cell
using all 5 runs is 3.9M in and 0.76M out. The Xo spec is 31k tokens of
every Xo prompt, so an endpoint with prompt caching cuts the Xo input cost
by most of that. Reasoning models add their hidden reasoning tokens to the
output count. Multiply by the provider's price per million tokens.
