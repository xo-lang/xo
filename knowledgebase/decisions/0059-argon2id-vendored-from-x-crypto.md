# 0059: Argon2id in std/crypto, vendored from golang.org/x/crypto

Status: Accepted (2026-10-08). Supersedes the "scrypt or Argon2id from
`golang.org/x/crypto`" alternative of 0056.

## Context
Decision 0056 shipped PBKDF2-HMAC-SHA256 as the only password hash because
the compiler and runtime were standard library only. PBKDF2 is not memory
hard; OWASP recommends Argon2id first. Go's standard library has no Argon2.
The implementation lives in `golang.org/x/crypto/argon2` (with
`x/crypto/blake2b`, and `x/sys/cpu` for amd64 feature detection).

How the runtime reaches a build (`internal/build`, `internal/gen`): the Go
source of package `rt` (`std/rt/*.go`, embedded in the `xo` binary) is
copied into every program's build directory, and `gen.goBindings` writes the
program's `go.mod` (`module xoprog`, plus requirements only for `use go`
bindings). `goBuild` runs `go build` with `GOFLAGS=` and `GOWORK=off`, once
per `--targets` entry. `rt` is one package, so a dependency of any `rt` file
is a dependency of every Xo program.

## Experiment (2026-10-08)
A module requiring `golang.org/x/crypto v0.55.0` and calling
`argon2.IDKey` (tidy pulls in `golang.org/x/sys v0.47.0`):
- with the module cache populated and `GOPROXY=off`: builds and runs, and
  cross builds for `wasip1/wasm` and `windows/arm64`;
- with an empty `GOMODCACHE` and `GOPROXY=off`: fails with `module lookup
  disabled by GOPROXY=off`.

So the dependency route works offline only on machines that already
downloaded the modules. Because `rt` is shared, a fresh install without
network could not build any program, even one that never hashes a
password. The compiler module itself (whose `std/rt` tests compile `rt`)
would also gain its first third party requirement.

## Decision
Vendor, do not depend:
- `std/rt/xcrypto_argon2.go`: `argon2.go`, `blake2b.go`,
  `blamka_generic.go`, `blamka_ref.go` of `x/crypto/argon2` v0.55.0,
  renamed into package `rt` (`argon2*` identifiers) by a script; the
  exported `Key` and `IDKey` are replaced by `argon2Key(mode, ...)`.
- `std/rt/xcrypto_blake2b.go`: the unkeyed digest and generic compression
  of `x/crypto/blake2b` (`b2b*` identifiers).
- Both keep the Go authors' BSD 3 clause license text in the file header.
  Only portable Go is kept: no assembly, no `x/sys/cpu`, so every
  `--targets` entry builds the same code.
- API (std.md 11): `crypto.hash_password_argon2id(r: Rand, password,
  memory_kib = 19456, iterations = 2, parallelism = 1) -> Str uses rand`,
  OWASP minimum defaults, 16 byte salt, 32 byte key, PHC string
  `$argon2id$v=19$m=..,t=..,p=..$salt$key`. `verify_password` dispatches on
  the prefix (`$argon2id$` or `pbkdf2-sha256$`). Argon2id is the
  recommended default in the spec; `hash_password` (PBKDF2) stays.
- Verification: RFC 9106 section 5 vectors for Argon2d, Argon2i, and
  Argon2id (with secret and associated data), RFC 7693 BLAKE2b-512("abc"),
  and 30 random cases whose keys were recorded from `x/crypto`'s `IDKey`
  (`std/rt/testdata/argon2id_xcrypto.txt`), plus a PHC string made with
  `x/crypto` that `verify_password` accepts.

## Consequences
- No new module requirement for the compiler or for generated programs;
  builds stay offline and reproducible on a fresh machine, for every target.
- Speed: one hash at the defaults takes about 19 ms on Apple M (arm64,
  where x/crypto also runs the generic code). On amd64, x/crypto would use
  its SSE4.1 assembly and be faster; the vendored copy is portable Go
  everywhere. Acceptable for login rates; revisit if amd64 servers need it.
- Updates are manual: diff `x/crypto/argon2` and `x/crypto/blake2b` of a new
  release against the vendored files, regenerate, and rerun
  `go test ./std/rt -run 'Argon2|Blake2b'`. Argon2 and BLAKE2b are fixed by
  RFCs, so changes upstream are expected to be rare (performance, not
  output).
- `verify_password` bounds the stored parameters (lanes 1..=255, memory up
  to 4 GiB, passes 1..=1000, key up to 1024 bytes) so a tampered hash
  cannot request unbounded work.

## Alternatives
- Require `golang.org/x/crypto` in every generated `go.mod` (with a
  pinned `go.sum`): reproducible, but offline builds then need the module
  cache to hold `x/crypto` and `x/sys` (shown above), for every program.
  Rejected.
- Require it only when a program uses `hash_password_argon2id`: needs `rt`
  split so the Argon2 file is copied conditionally, and the program's
  dependencies would change with one call. More moving parts than a
  vendored file. Rejected.
- Bind it with `use go` (decision 0041) from user code: works today, but
  then the recommended password hash needs Go interop and a module
  download. Rejected for std.
- scrypt (also `x/crypto`, less code to vendor): OWASP lists it after
  Argon2id. Rejected.
