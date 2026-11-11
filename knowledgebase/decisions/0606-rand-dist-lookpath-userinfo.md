# 0606: Random distributions, look_path, and URL user info as free functions and defaulted fields

Status: Accepted (owner, 2026-11-11; Xo Lang 3)

## Context

The std audit of 2026-11-11 (gaps 17 and 30, and the `url.Userinfo`
rows of scripts/stdaudit/map.txt) left three Go features whose Xo shape is
a choice, not a port:

- Go's `rand.NormFloat64`, `ExpFloat64` and `Perm`.
- Go's `exec.LookPath`: finding a command on PATH without running it
  (`Proc.cmd` searches PATH only when it starts the command).
- Go's `url.Userinfo`: `user:password@` in a URL. `http.Url`
  (`scheme`, `host`, `port`, `path`, `query`) has no field for it, so
  `parse_url` drops it today.

`Rand` and `Proc` are interfaces that a program's own type implements
(decision 0307). A new interface method breaks every such type, as
the compiler's own `RecFs` and `StatFs` would break for `Fs`. `shuffle`
avoids this only because the compiler derives it (`rt.Rand_shuffle`),
which takes compiler work on every backend. The same week,
`create_temp_dir` and `write_atomic` were added as free functions over
`Fs` for this reason.

## Decision

1. **A new module `std/random`** of free functions over any `Rand`:
   `normal(r: Rand) -> Float` (mean 0, standard deviation 1),
   `exp(r: Rand) -> Float` (rate 1), and `perm(r: Rand, n: Int) ->
   List[Int]`. They port Go's ziggurat tables (math/rand/v2 normal.go,
   exp.go) and draw through `r.int` and `r.float` alone. They work on a
   test's FakeRand and on a program's own Rand. A value matches Go's
   only for a matching source, so the corpus checks the tables and the
   transformation of a fixed stream of draws. It also checks the moments
   over 10^6 draws.
2. **`look_path(env: Env, fs: Fs, name: Str) -> Str ! ProcErr`** in the
   prelude, with Go's rules: a name with a `/` is tried as it is; PATH
   from `env` is split on `:` (`;` and PATHEXT on Windows); an empty
   PATH entry is `.`. The first regular file with an execute bit wins.
   A result relative to the working directory is an error, as Go's
   `ErrDot` is (`ProcErr.NotFound` with Go's text). It takes `Env` and
   `Fs` instead of `Proc`, so the `Proc` interface is unchanged and an
   attenuated `Fs` limits what it can see.
3. **`http.Url` gains `user: Str = ""` and `password: Str? = None`**, as
   defaulted fields (as `FileInfo.is_symlink`), so every existing
   literal still compiles. `parse_url` fills them (percent decoded, Go's
   `url.Parse`). `display` writes them back escaped as Go's
   `Userinfo.String` does. A request to a URL with a user sends
   `Authorization: Basic` unless the request sets its own, as Go's
   Transport does. `Url.redacted()` shows `xxxxx` for the password (Go's
   `URL.Redacted`).

## Consequences

- Programs no longer write their own Box-Muller or shuffle a range
  for `perm`; the audit's three rows map to std.
- `std/random` is a new module name (std.md gains a section).
- A URL's password reaches `display`. Logs should use `redacted()`.
  std/changes/http.md says so.
- None of the three changes an interface, so no user capability type
  breaks.

## Alternatives

- Methods on `Rand` (`r.normal()`): either every user Rand must add
  them, or the compiler derives them as it does `shuffle` (check, SSA
  and each backend). Rejected for the cost, though it reads better. If
  the owner prefers it, item 1 becomes compiler work in a later stage.
- `Proc.look_path(name)`: breaks user Proc types. It would also see the
  real file system even when the program holds only an attenuated
  `Fs`.
- A separate `UserInfo` struct (Go's shape): one more type for two
  strings. Defaulted fields keep `Url` flat as it is today.
- Prelude free functions `rand_normal` and the like instead of a
  module: they put three Rand helpers in every program's namespace.
