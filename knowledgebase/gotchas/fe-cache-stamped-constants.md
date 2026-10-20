# A stamped constant replayed another build's value from the front end cache

Status: Observed (2026-10-20, found by Xo lang 5's TestVersionStamp
failure; fixed the same day, Xo Lang 3).

## Symptom

An xo built by internal/xotool with its own `--stamp` values (commit
9a70c1c1, a source key) reported the fixed point bootstrap's stamps
(commit 7a1807c4, "built by bootstrap-xo.sh", no source key):
TestVersionStamp failed in -short. The minimal form: a program with
`const VERSION: Str = "dev"` read in a function body, built twice in one
cache with `--stamp VERSION=one` then `--stamp VERSION=two`, prints
`version one` both times, on arm64 and LLVM; with XO_FE_CACHE=off it
prints `version two`.

## Cause

`--stamp` (decision 0264, compiler/xo stamp.xo) replaces the constant's
literal in the checked syntax tree after the check. The front end
cache keys a program body by its source text and its module's interface
closure (the files as written), so the stamped value was in no key and
the first build's body replayed. Any xo building one program twice with
different stamps hit it; decision 0275 made it reach across xo builds
(entries shared by every xo with the same code generation key), which is
how a stamped xo picked up the bootstrap's stamps.

## Fix

compiler/xo bcache.xo stamp_root: a program body's key also holds a
hash of the build's stamps (passed as part of fe_keys' root, which only
program bodies' keys read; std bodies never see stamps).
internal/arm64 TestFrontCacheStamps builds a stamped program three times
in one cache on arm64 and LLVM. Rule: anything that rewrites the syntax
tree after the check is an input of the front end cache's keys.
