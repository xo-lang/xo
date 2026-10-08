# The original agent benchmark prompts were not saved

Status: Observed (2026-10-08, packaging the benchmarks in `bench/harness/`)

Symptom: rerunning the stage 3 and planted bug benchmarks with another
model needs "the same prompts as the original runs", but no prompt text
exists anywhere in the repo or the git history. Only their effects are
recorded: `BUILD_LOG.md` and `NOTES.md` conventions, `bench/README.md`
stage 3, `bench/planted/README.md`, and the result files.

Cause: the subjects were Claude Code subagents; their prompts lived only
in the orchestrating session's transcript.

Fix: `bench/harness/prompts/` now holds reconstructed prompts, with every
known difference listed in `bench/harness/README.md` ("Protocol and
differences from the original runs"). For any new agent run, save the
exact prompt next to the outputs (the adapter writes the whole
conversation to `transcript.jsonl`; for Claude Code subagents, commit the
prompt as `PROMPT.md` in the run directory) before reporting results.
