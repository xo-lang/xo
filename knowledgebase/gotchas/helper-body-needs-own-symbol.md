# A helper's shared body needs its own symbol in a split build

Status: Observed (2026-11-04, Xo Lang 1).

## Symptom

On darwin, an edit build of separately compiled halves (decision 0409,
internal/arm64 TestUnwind's sephalf case, and the hourly rss-guard build)
failed to link: `xo: link: ...prog-darwin.1.o: no FDE for the DWARF mode
function at ADDR`. Ordinary builds linked.

## Cause

The drop thunk (compiler/arm64/rc.xo drop_thunk, 522329233) is 28
two-instruction entries that branch to one shared body. Each entry got a
global alias (alias_sub) in a split build, the body only a local label.
The Mach-O link splits a section into atoms at global symbols, so the body
became the tail of the last entry's atom; its own `.cfi_startproc` region
(a frame record, so DWARF mode) then had an unwind entry whose function
address no FDE started at.

## Fix

Name every `.cfi_startproc` region of a helper with alias or alias_sub,
not only its entry points: the body now has `alias_sub(key, "body")`. Run
`go test ./internal/arm64 -run TestUnwind` (it covers the sephalf link) for
a helper with more than one region.
