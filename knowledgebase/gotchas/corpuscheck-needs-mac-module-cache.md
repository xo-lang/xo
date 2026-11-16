# corpuscheck fails on a Linux offload host, with no stale line

Status: Observed (Xo Lang 3, 2026-11-16, three gate rounds of l3apicov)

## Symptom

`offload-gate.sh WT ... corpuscheck` on host 2 or 3 ends with
`corpuscheck: FAIL: a corpus differs from its generator (...)` and no
`stale` line after it, while `go run ./scripts/stdcorpus -check` on the
Mac passes on the same commit. The host deletes the step's log when the
gate ends, so it cannot be read afterwards.

## Cause

The oracles (scripts/stdcorpus/*oracle, their own modules) read their Go
modules from the module cache with GOPROXY=off; the Mac's cache has them,
an offload host's does not, so a generator fails to build there
(scripts/gate.sh's corpuscheck comment: the step runs where they are, the
Mac's tier steps).

## Fix

Run corpuscheck on the Mac: `HEAVY_CLASS=gate ~/xo-lead/agents/heavy.sh
NAME scripts/gate.sh corpuscheck` in the worktree, and gate the other
steps on a host. A real stale corpus prints `stdcorpus: PATH is stale`.
Also seen the same day: two corpora on main were stale (xml after
614956711, tabwriter after 292f855ef hand-edited a generated file); pushq
#140 now runs corpuscheck for a corpus change and xo-guard #138 blocks
hand edits of generated files.
