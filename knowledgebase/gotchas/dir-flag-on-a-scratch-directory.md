# `--dir` on a scratch directory loaded every tree under it

Status: Observed (2026-10-21)

**Symptom.** `xo doc --dir=/private/tmp Map` ran for 55 minutes and held 223
GB (220 GB compressed): memory stayed near 31% free and every gate waited.

**Cause.** A `--dir` with no .xo file directly in it is a workspace root:
doc, def, and refs check every module under it. /private/tmp held every
agent's scratchpad and worktree copies.

**Fix.** agent.xo too_wide walks the directory first (bounded) and refuses
one with more than 6000 .xo files or 60000 entries, naming the limit. The
xo checkout itself has about 1,800 .xo files. Test: cmd/xo
TestDocRefusesWideDir.

**Avoid.** Give --dir a module or repository directory, never /tmp, a
home, or a scratchpad; check how much a command will load before pointing
it at a parent directory.
