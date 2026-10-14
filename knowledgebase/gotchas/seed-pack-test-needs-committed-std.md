# TestSeedPackHello fails on a new std file that is not committed

Status: Observed (2026-10-14, work item B21)

Symptom: `go test -short ./internal/xotool` failed in TestSeedPackHello
with `unpacked seed running hello: ... error XO0601: module
native/llrt/heapos not found` while every other test, and the xo built
from the same tree, passed.

Cause: scripts/seed-pack.sh packs std with `git ls-files -- std`, the
committed files only (a seed is cut from a commit). A new runtime module
that the tree's bin/xo already uses but that is still untracked is
missing from the packed std, so the unpacked seed cannot build hello.

Fix: commit (or at least `git add`) the new file before running the
-short suite; nothing is wrong with the change itself.
