# `requires` is a keyword, so it cannot name a field

Status: Observed (2026-10-10)

## Symptom

Porting `mod.File.Requires` as a struct field `requires: List[Require]`
gave XO0160 `expected a field name, found keyword requires` at the
declaration and at every `f.requires`, plus confusing follow-on errors
(XO0403 `missing field(s) `` in DepState literal`, unused loop variables).

## Cause

`requires` and `ensures` are contract keywords (core 8), reserved
everywhere, including after `.`.

## Fix

Name the field something else (`reqs`, `req_cache` in
compiler/check/modfile.xo). When a port of Go code shows a burst of XO0160
and XO0403 with an empty field name, look for a Go identifier that is an
Xo keyword.
