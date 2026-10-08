# Go's flag package stops at the first positional argument

Status: Observed (2026-10-08, `cmd/xo`)

Symptom: `xo query type api/users.xo:18:7 --json` would treat `--json` as
a second argument, because `flag.FlagSet.Parse` stops at the first
argument that is not a flag.

Cause: the standard library flag package (unlike most CLIs) requires
flags before positional arguments.

Fix: `xo query` scans its arguments itself (`--json`, `--text`,
`--dir=`, anywhere after the kind). The other subcommands use `flag` and
need their flags first (`xo fmt --check dir`, `xo doc --all mod`).
