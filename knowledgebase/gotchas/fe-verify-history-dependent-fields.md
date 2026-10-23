# XO_BACKEND_CACHE=verify flagged front end entries another program recorded

Status: Observed (2026-10-22, fixed 2026-10-23).

## Symptom

`XO_BACKEND_CACHE=verify` failed with "N function bodies' SSA differs
from their front end cache entries" (prelude.llrt_str_hash,
http.call_raw, ...) on 297 of internal/arm64's goldens against the user
cache, and on a private cache primed by one xo, while every binary was
byte-identical with the cache off, cold, and warm. gate.sh's warm-verify
step failed on main for days, so it said nothing about the change it
gated.

## Cause

An entry records two things that depend on what the build did before
the body, not on the body:

- the tail's module list: the modules the body was the first to build
  code of (decl_mods only grows), so a body recorded after another
  declared native/llrt/ints lacks it;
- closure and constant numbers (NAME.fnN, rt.const_once's id): program
  wide counters, which a replay renumbers as the build here numbers
  them (fe_replay's renames, fe_const_ids).

A replay declares the body's functions again (their modules follow) and
renumbers, so the build is the same; verify compared the raw bytes.
Two primings of one program give identical entries, which is why a
single program never showed it.

## Fix

Verify compares with `Builder.fe_same_numbered` (compiler/ssa
fecache.xo): the tail without its module list (tail_sans_mods), and the
bodies decoded with closures numbered by first use and constant ids
zeroed. TestGoldenRun with XO_BACKEND_CACHE=verify: 297 failures, now 0.
To look at a mismatch, dump the pair (old entry, fresh entry) where
build.xo pushes fe.mismatches and diff the bytes around the first
difference.
