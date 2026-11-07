# 0547: std cli accepts single-dash long flags on an opt-in command

Status: Accepted (owner, 2026-11-07; proposed by Xo Lang 2, 2026-11-07, at the lead's request)

Extends decision 0160 (std cli grammar, point 5).

## Context
Decision 0160 chose GNU style for std cli: `--name` for long flags, `-n`
for a short one, `-vq` for a cluster. It named Go's flag package
(`-name` long flags, no shorts, stop at the first positional) as the
rejected alternative. Porting goversioninfo (owner-approved, 2026-11-07),
a widely used Go tool, showed the cost: every documented invocation,
`//go:generate goversioninfo -icon=icon.ico -manifest=x.manifest -64`,
fails under std cli with `unknown flag -i` (exit 2), because `-icon` is
read as the cluster -i -c -o -n. A port that changes its command line
breaks every build that calls it, so a Go tool's port could not use std
cli (the alternative, each port writing its own parser, defeats std cli).

## Decision
`cli.command` takes `single_dash_long: Bool = false`. On a command that
sets it, and every subcommand under it:

1. An argument `-word`, `-word=value` (one dash, before `--`) is read as
   `--word...` when word is a declared long flag of the path, `help`, or
   `version` (when the root has a version). A bool flag takes
   `-name=false` as `--name=false` does; a valued flag takes its value
   after `=` or as the next argument.
2. Otherwise the argument is a short cluster, as 0160 reads it. A word
   of more than one letter that is neither a long name nor a valid
   cluster is the usage error `unknown flag -word`, with a did you mean
   in the `-` form.
3. Ambiguity: a declared long flag wins over a short cluster. A long
   flag whose name is also a valid cluster of the path's declared shorts
   (each letter a bool short until a short that takes a value, which
   takes the rest; `-h` counts) is a declaration fault at parse, `cli:
   flag --NAME also reads as short flags -NAME under single_dash_long
   (command C)`, except a one letter long flag whose own short is that
   letter (`-o` means the same flag both ways).
4. Flags still come before or after positional arguments; Go's stop at
   the first positional is not copied (a documented difference). Help
   text keeps the `--` forms.

## Consequences
- goversioninfo's port keeps every Go invocation (`-icon=x -64 -o out`),
  and `--icon` works too.
- Off by default; commands without it are unchanged.
- The fault in point 3 makes a declaration choose: a tool with short
  `-o` (valued) cannot also have long flags starting with `o` under the
  option. A Go tool has no shorts, so a port declares none.
- Implemented in std/native/cli.xo and std/rt/cli.go; golden
  cli_single_dash and cli_single_dash_fault on every backend.

## Alternatives
- **Each port parses its own flags**: the lead rejected it (2026-11-07):
  ports are how Xo is tested as real programs use it, and std is what
  they should use.
- **Read `-word` as long whenever it matches, without the setup fault**:
  silent ambiguity; `-out` would mean `--out` or `-o ut` depending on
  which flags exist, and adding a long flag later would change what an
  existing command line means.
- **A separate `go_flags` mode copying Go exactly** (stop at the first
  positional, no shorts, Go's usage text): more surface, and the
  positional rule is worse for users.
