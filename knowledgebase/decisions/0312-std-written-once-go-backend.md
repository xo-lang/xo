# 0312: The Go backend compiles the pure-Xo std modules (std written once, step 2)

Status: Accepted (owner, 2026-10-21): the Go backend's slower calls
accepted, item 4's escape hatch kept. Drafted by Xo Lang Lib 1 the same
day.

Order (owner):
1. Port Go's path.Clean algorithm natively. The profile of the
   measurement below put about 80% of the 4x in path.xo's algorithm (it
   splits into a list and joins), not in gen.
2. Pilot: path, then uuid.
3. Then the rest, one module per push, each after its corpus, with
   goldens identical on go, arm64, and LLVM.

Once the Clean port and Xo Publish's lazy list blocks have both landed,
the path measurement is rerun.

## Context

A std module has three parts today:

- `std/M.xo` declares it.
- `std/native/M.xo` implements it for the native backends.
- `std/rt/M.go` implements it again, by hand, for the Go backend, mostly
  by calling Go's own package.

Decisions 0164 and 0172 set up that split for their modules. The later
module decisions (0276's ports, 0282 multipart, and others) follow the
same pattern. It gives the native port an oracle: run the program on the
Go backend, where Go's package does the work, and compare. Most of
2026-10-21's std bugs were found that way (parse_url, Set-Cookie, the
client's ReadResponse, untar, unzip).

The cost is two implementations of every module. Step 1 of "std written
once" (bdd73dd8, one generated file pair per module) removed the merge
conflicts in the generated files. The hand-written std/rt files remain,
and the two implementations can drift in small ways that only a
differential finds.

Since 2026-10-21 the oracle has a second form that does not need the Go
backend: corpora generated from Go itself. `scripts/stdcorpus MODULE`
runs Go's package on Go's own test inputs and on seeded random ones and
writes the results as an Xo test
(internal/stdtest/testdata/corpus/MODULE). `-check` keeps each corpus
reproducible from its generator. TestStdCorpora runs the corpora on every
native backend outside -short. On their first runs the corpora found:

- mail: 2 native differences;
- compress: the order of errors;
- json.Decoder: 3 differences;
- multipart: a skipped quoted-printable part.

## Decision

1. **Pure-Xo modules compile on the Go backend.** A std module whose
   `std/native/M.xo` imports only the prelude and other std modules (no
   `native/llrt`, no `rt.*` intrinsics, no `use c`) is compiled by
   compiler/gen from that file, as it compiles a user module.
   - Its `std/rt/M.go` is deleted.
   - scripts/stdgen emits only its types for the Go backend.
   - stdgen records the pure modules in a generated list, and `stdgen
     -check` fails when a module on the list gains a native-only import.
2. **The modules** (13, as of 2026-10-21): base32, base64, binary, mail,
   mime, multipart, netip, path, smtp, template, term, url, uuid.
   - Together their std/rt files are about 1,780 lines.
   - Their native files are about 7,300 lines and stay the single
     implementation.
3. **A module moves only once it has a generated Go corpus.** It needs a
   `scripts/stdcorpus` generator (reproducible, under `-check`) that
   covers its public API with Go's results.
   - As of 2026-10-21 (11:45): base32, base64, binary, mail, mime,
     multipart, netip, template, and url have one, and with path's
     one-off table that is ten of the thirteen. base32, base64,
     binary, mime, netip, path, template, and url have Go-derived tables
     in their std tests, made by one-off programs; these become stdcorpus
     generators first. smtp and term have none. term has no Go package
     to compare with, since its API is Xo's own (colors, NO_COLOR): its
     module tests stay its oracle. smtp needs a scripted SMTP server for
     both net/smtp and the Xo client to talk to (FakeNet), which is its
     corpus to build.
   - uuid's oracle is github.com/google/uuid, which is not Go's standard
     library. Its generator may keep using it, as its test does now.
   - The corpus runs on the Go backend too (TestStdCorpora gains `go`),
     so a gen bug in a moved module is caught the same way.
4. **What stays hand-written in std/rt:**
   - the prelude;
   - every module that wraps Go runtime facilities or needs native-only
     code: http, sql, crypto, tls, time, testing, cli, log, task,
     internal, archive, compress, csv, json, regexp, toml;
   - the modules whose Go implementation is meant to be Go's (regexp,
     json), which keep Go's speed on the Go backend.
   - A module leaves this list only by a later decision.
5. **Order.** path and uuid first, as the pilot. Then base32, base64,
   binary, url, netip, mime, mail, multipart, template, smtp, term, each
   after its corpus. Each move is one push, and the module's goldens must
   stay identical on go, arm64, and LLVM.

This supersedes the part of 0164 and 0172, and of the module decisions
that follow them, that fixes how these 13 modules run on the Go backend.
The oracle rule is unchanged: Go's results stay the reference, but they
come from the generated corpora, not from the Go backend calling Go's
package. 0164's and 0172's own modules (csv, toml, regexp, archive) are
not affected.

## Consequences

- Implementation notes (2026-10-21):
  - **Order, step 1.** path.Clean is Go's algorithm (5cb4ffe1). Measured
    at 300,000 iterations, medians of 5 on a busy machine:
    - program b, gen-compiled path.xo on the Go backend, went from 0.20 s
      to 0.13 s, against 0.05 s for Go's package;
    - the native loop went from 0.18 s to 0.08 s.
  - **Pilot, path.**
    - The list is std/go_compiled.txt, read by the checker (load.xo
      go_compiled, load_go_compiled) for the Go backend only.
    - gen's emit_call routes a listed module's free function to the
      implementation's function (call.xo go_impl), and the implementation
      module is compiled as a Go package (x_native_path).
    - stdgen writes no signature checks for those functions, and
      std/rt/path.go is deleted.
    - Goldens std06_path (Go backend) and std_path (Go parity, arm64,
      LLVM) pass.
  - **Methods (uuid).**
    - A std method that is not static, var, generic, or an interface's
      maps to the implementation's free function TYPE_method (natively
      `uuid_version`). It is called with the receiver as its first
      argument (call.xo impl_member).
    - `display` and `debug` stay Go methods in std/rt, since the runtime
      calls them through interfaces. std/rt/uuid.go is now just those.
    - The uuid corpus (google/uuid) passes on go, arm64, and LLVM, and
      uuid_basic passes with Go parity.
  - **Moves after the pilot** (Go backend timing: Go's package, then the
    module's own rt code, then std/native/M.xo compiled by gen; medians
    of 5, the same loop and output; MODULE_BENCH is
    ~/xo-lead/notes/lib1/mvbench.sh):
    - base32 (2026-10-21): 1 KB encode and decode, 20,000 times: Go's
      package 97 ms, rt wrapper 86 ms, native Xo through gen 538 ms
      (native arm64 312 ms). The rt copy was Go's package itself; the
      cost is gen's code for byte loops (Xo Publish's profile items).
    - base64 (2026-10-21): 1 KB encode and decode, 20,000 times: Go's
      package 53 ms, rt wrapper 37 ms, native Xo through gen 593 ms
      (native arm64 180 ms); 11 times Go's package.
      After the decode table and fast path (std/changes/base64.md): native
      33 ms (Go's package 51 ms), Go backend 172 ms (3.4 times).
    - base32 after the same fix (std/changes/base32.md): native 74 ms
      (Go's package 61 ms), Go backend 227 ms (3.7 times).
    - **Owner decision (2026-10-21):** the 3.4 and 3.7 times are accepted
      (option a): base32 and base64 stay compiled from Xo on the Go
      backend, and the escape hatch (a std/rt file and no entry in
      go_compiled.txt) stays unused. The first measurement, 11 times for
      base64, was the native code's per-quantum lists, not gen's code; a
      module is profiled natively (XO_PROF=cpu) before it moves, and its
      Go-backend ratio goes in the move's commit message.

- One implementation per pure module. A fix lands once and reaches every
  backend.
- The Go backend stops being an independent oracle for these modules:
  the native-against-Go differential finds nothing for them. The
  corpora carry that role. They must be at least as thorough as the
  differentials were, which is why a module moves only after its
  generator exists.
- Measured on path (2026-10-21, Mac arm64): a 300,000-iteration loop of
  join, clean, base, ext, and dir.
  - Program a: `use std/path` on the Go backend, so Go's path package.
  - Program b: the same program, with std/native/path.xo copied into a
    user module and compiled by gen. That is what this decision does.
  - Results, as one command after the other on an idle machine:

    | | a: Go's path | b: gen-compiled path.xo |
    |---|---|---|
    | output | 4300000 | 4300000 |
    | run time (5 runs) | 0.03 s | 0.12 s |
    | binary size | 5,233,170 B | 5,266,722 B (+33.5 KB, +0.6%) |
    | build, warm (2 runs) | 0.18 to 0.19 s | 0.17 to 0.18 s |

    The first build of each (1.61 s and 0.71 s) included one-time cache
    fills, so it is not compared. Script:
    ~/xo-lead/notes/lib1/swo-measure.sh.
- **Speed on the Go backend.** gen's code for a string-heavy module is
  about 4 times slower than Go's hand-tuned package: about 60 ns more per
  call here. The native backends are unaffected, because they already run
  the Xo code. Go backend programs that call these modules in hot loops
  get slower. Two options:
  - accept it, since the Go backend is not the performance target
    (decision 0215);
  - keep a module on Go's package for speed, which is item 4's escape
    hatch. Its oracle stays the corpus.
- Size grows by the module's generated code (here +0.6%). Build time does
  not change measurably.
- smtp and term perform I/O through Net and Stdio. Compiled by gen, they
  use the Go backend's capability objects as user code does, so fakes
  (FakeNet, test stdio) behave the same on every backend.

## Alternatives

- **Keep both implementations (status quo).** Two copies to keep equal,
  and every new module costs a second port. The independent oracle is
  its one benefit, and the corpora now provide it.
- **Generate std/rt Go from the native Xo with a separate tool.** That is
  what compiler/gen already is. Running it on std as on user modules
  needs no second translator.
- **Move every module, including regexp and json.** Their Go
  implementations are Go's own optimized packages. Through gen they would
  be slower, and their oracle is the package itself.
