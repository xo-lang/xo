# 0351: Warm builds without per-directory loose reads

Status: Proposed (Xo Lang 2, 2026-10-23). Measurements and the design
follow; the lead approved the investigation, the owner decides.

## Context

A warm `xo build` of a one-line program (darwin/arm64, xo at 923aed0a4,
a warm backend cache; ~/xo-lead/notes/xo-publish/warm-build-profile.md):

- in a working directory whose packs exist: real 0.30 s, user 0.65 s,
  sys 0.13 s;
- in a new working directory, same cache: real 0.38 to 0.55 s, user
  0.5 s, sys 0.84 to 0.92 s on each of its first three builds; XO_PROF
  puts 65 to 70% of the samples in `open`.

Packs are keyed by the working directory plus the command's words
(bcache.xo pack_dir, so one program's segments do not hold another's).
Until a directory's packs form, every entry the build reads (parse, fe,
borrow, rc, inl, sum, fn, obj: about 6,000 files for hello, almost all
of them std and runtime functions every program shares) is a loose file
opened in parallel. Every caching layer hits on such a build; the time
is the opens.

## Decision

To be written with the measurements: a pack keyed by content for the
entries every program shares (std and runtime), read by every directory
and program, beside the per-directory pack; and, as a second stage, a
whole-build key (Go's action IDs) that reuses the linked binary when
nothing that decides it changed.

## Consequences

To be written.

## Alternatives

To be written.
