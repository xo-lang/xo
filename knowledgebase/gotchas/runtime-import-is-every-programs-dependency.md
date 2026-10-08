# A third party import in std/rt is a dependency of every Xo program

Status: Observed (2026-10-08, adding Argon2id to std/crypto, decision 0059)

Symptom: after a std/rt file imports a module outside the Go standard
library (for example `golang.org/x/crypto/argon2`), `xo build` of any
program, even one that never calls the new function, needs that module.
On a machine whose module cache lacks it and that has no network, `go
build` fails with `module lookup disabled by GOPROXY=off` (or a download
error), reported by xo as a code generator bug (`BuildError`).

Cause: internal/build copies all of `std/rt/*.go` into every build
directory as one package `xoprog/rt`, and the generated `go.mod` is
`module xoprog` plus only the requirements of `use go` bindings. There is
no per program selection of runtime files, so an import in one rt file is
an import of every program. The compiler module (`std/rt` tests) would
need the requirement too.

Fix: do not import third party modules from std/rt. Vendor the needed
source into package rt as renamed identifiers with the upstream license in
the file header (as `xcrypto_argon2.go` and `xcrypto_blake2b.go` do), keep
only portable Go so every `--targets` entry builds, and pin the result
with known answer tests. A library that users opt into belongs behind
`use go` (decision 0041), whose requirements go only into the programs
that bind it.
