# 0556: std cli can end flags at the first positional argument, as Go's flag package

Status: Proposed (Xo Lang 2, 2026-11-07, at the lead's request; for the owner to decide)

Extends decisions 0160 (std cli grammar) and 0547 (single-dash long flags).

## Context

Decision 0160 reads flags anywhere on the command line: `tool a.json -v`
sets `-v`. Go's flag package stops at the first argument that is not a
flag; everything after it is positional (`flag.Args()`), so
`goversioninfo versioninfo.json -64` treats `-64` as a second file name
and ignores it, while the Xo port, on std cli with decision 0547's
`single_dash_long`, reads it as the flag and builds 64-bit resources.
0547 kept this difference on purpose (its point 4) and named the Go rule
"worse for users". The goversioninfo port's FEEDBACK.md item 1 lists it
as the one remaining command line difference from the Go tool; the lead
routed it here (2026-11-07). A port that should behave as the Go tool
for every argument list, the rule ports follow, needs Go's rule.

## Decision (proposed)

`cli.command` takes `stop_at_positional: Bool = false`. On a command that
sets it, and every subcommand under it:

1. The first argument that is not a flag, a flag's value, or (before any
   positional) a subcommand's name ends the flags of the whole line: it
   and every argument after it are positional, read by the command's
   `arg`s and `rest` as written (`-64` included). `--` still ends the
   flags too, and is dropped as today.
2. A lone `-` is positional (as in Go and as 0160 reads it), so it ends
   the flags as well.
3. Arguments after the stop that look like flags are never usage errors;
   extra positionals without a `rest` are 0160's `unexpected argument`.
4. Help text says so in the usage line (`[flags] ARGS...` with "flags
   come before the arguments" under it), so `--help` explains it.
5. Independent of `single_dash_long`; a Go port sets both.

## Consequences

- goversioninfo's port reads `versioninfo.json -64` as Go does; its
  corpus generator can place flags after the file name.
- Off by default; commands without it are unchanged.
- Implemented in std/native/cli.xo and std/rt/cli.go with a golden
  (cli_stop_positional) on every backend and Go parity, spec/std.md's
  cli section and std/changes/cli.md.

## Alternatives

- **Keep the difference** (0547's point 4): a port then differs from its
  Go tool on any line with a flag after a file name; ports exist to
  behave as their tools.
- **Make it part of `single_dash_long`**: one option for "Go style", but
  a tool may want `-name` flags with flags anywhere, and the two rules
  are separate questions.
- **A port-side rewrite of argv before parsing** (move everything after
  the first positional behind a `--`): works without std, but every port
  repeats it and has to know which arguments take values.
