# 0095: Go backend output size: protocol only runtime generics, thin package main, stripped binaries, net/http on demand

Status: Accepted (2026-10-09; task from the owner with targets: hello world
at or below 3 MB, per program Go cache growth at or below 5 MB, compile
RSS at least halved)

## Context

Measured 2026-10-09 (bench/results/binary-size-2026-10-09.md): hello world
on the Go backend was a 7.9 MB binary (plain Go `fmt.Println` 2.4 MB) and
added 34 MB to the Go build cache per program, with 280 MB of compiler
RSS for each of its two packages. Test suites build hundreds of programs,
so the cache grew past its cap within the trim guard and parallel suites
ran out of memory. Cause (gotcha `go-generic-method-sets-per-package.md`):
Go compiles every method of every generic instantiation a package reads,
in that package, and the runtime's API reaches about 80 instantiations
from `rt.Task`. Binary size came from DWARF and the symbol table (a third)
and from net/http, which rt imports for std/http and which brings
crypto/tls, x509, mime, gzip, and idna into every program.

## Decision

1. Generic runtime types (List, Map, Set, Option, Result, Tuple2..6,
   Chan, Secret, json.Patch, the map's table) keep only the protocol
   methods runtime code calls through interfaces (`XoEq`, `XoHash`,
   `XoCmp`, `XoDebug`, `XoJsonEnc`, `XoJsonDec`, `XoJsonRead`, `XoGenAny`,
   `XoMark`, `M_display`, `M_debug`, the fx codec's). Every other
   operation is a package level function `<Type>_<name>`, which the
   generator calls (`rt.List_len[T](tk, xs)`, `rt.List_At(xs, i, loc)`).
   A test enforces it (`TestGenericMethodSets`).
2. Generated package main names no runtime type: it calls `XoMain` of the
   main module's package (not inlined), and test mains pass tables as
   `any` (`XoTests`, `rt.RunTestTables`).
3. Go backend builds leave out DWARF (`-gcflags=all=-dwarf=false`) and
   the symbol table and DWARF of the binary (`-ldflags=-s -w`) by
   default; `XO_GO_DWARF=1` keeps them for Delve. Nothing in xo reads
   either: stack traces, faults, crash bundles, `xo dump`, and pprof use
   the pclntab through the Go runtime.
4. A program that does not import std/http (directly or through std
   modules; test binaries always count as importing it) builds rt with
   the tag `xo_nohttp`, which replaces the net/http server and client
   (`http_net.go`) with unreachable stubs (`http_nonet.go`).

## Consequences

- Hello world: 7.9 MB to 3.6 MB, 34 MB to 2.9 MB of cache, 286 MB to
  123 MB compile RSS, 1.15 s to 0.24 s warm build; an HTTP service 12.8
  MB to 7.7 MB and 54 MB to 6.9 MB. Workload times unchanged
  (bench/perf/run.sh). The 3 MB binary target is not met; the next steps
  are in the results file.
- Std containers have no Go methods, so they satisfy Xo interfaces
  through generated adapters, and the `--no-mono` adapter registry
  records their instantiations (golden `std_iface_methods`).
- rt is compiled twice in a cache that sees programs with and without
  std/http (about 13 MB each).
- A panic or crash in a stripped binary still prints Go function names
  and files (pclntab); `go tool nm`, `addr2line`, and debuggers need
  `XO_GO_DWARF=1`.

## Alternatives

- Pre-instantiating in rt so importers reuse the code: Go has no way to
  export instantiations (the compiler's `importedDef` TODO), so every
  importer recompiles them regardless.
- Thin generic methods over non-generic bodies (`Gen[T]` boxing into a
  `genAny`): shrinks text but not the symbol count, relocations, and
  type descriptors that dominate the archives (measured: removing only
  the XoGenAny family saved 6 percent).
- One Go package for the whole program: halves package main's share but
  keeps 17 MB per program; superseded by item 2, which costs nothing.
- Keeping DWARF in development builds only: there is no build mode yet,
  and no xo feature reads DWARF.
- Splitting net/http (and pprof, encoding/json) into separate Go
  packages registered at init: a larger change for the same effect as a
  build tag; the tag needs no new package boundary through rt's
  internals.
