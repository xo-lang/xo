# Binary size: Xo native, Go, and Rust (2026-10-16)

The same four programs written idiomatically in Xo, Go, and Rust, built
every supported way, on darwin/arm64 and linux/arm64, then a breakdown of
where an Xo arm64 binary's bytes go.

## Setup

- Xo: main at 8813beb0 (xo 0.17.0), `--backend=arm64` (macho linker, the
  darwin default) and `--backend=llvm`, each with and without `-s`,
  `XO_BIN_CACHE=off`. Plain `xo build` still uses the Go backend (6.8 MB
  for https_get) and is not in the tables.
- Go: 1.26.1 on the Mac, 1.26.9 in Docker; default and
  `-ldflags="-s -w"`. notes uses golang.org/x/crypto v0.57.0 (Argon2id).
- Rust: 1.99.1, `--release` (opt-level 3), and the same with
  `strip = true`. Crates pinned in Cargo.lock: ureq 3.4.2 over rustls
  0.23.46 with ring 0.17.14 (https_get), tiny_http 0.12.0 and serde_json
  1.0.154 (json_server, notes), argon2 0.5.3, sha2 0.10.9, chrono 0.4.45,
  ctrlc 3.5.2 (notes). Rust had no setup in bench/perf to reuse, so it is
  new here.
- Hosts: Apple M4 Pro, macOS 27.0; linux/arm64 in Docker
  (golang:1.26-trixie via scripts/docker-linux.sh, xo cross built for
  linux/arm64 on the Mac).
- Programs (bench/size/native; notes is examples/notes-service, ported
  to go/notes and rust/notes):
  - hello: print one line.
  - https_get: GET https://example.com/ (or $URL), print the status and
    the body's length.
  - json_server: GET /items, GET /items/{id}, POST /items with JSON
    bodies over a list under a lock.
  - notes: bearer token auth, SHA-256 token digests compared in constant
    time, Argon2id passwords and POST /login, expiring notes with a
    background purge, two phase shutdown. Go and Rust have no SQL engine
    in std, so their ports keep the rows in maps under a mutex (Xo uses
    std/sql's in memory driver, 125 KB of its code).
- Every darwin binary was smoke tested (hello prints, https_get gets 200,
  json_server answers POST and GET, notes answers /health, creates a
  note, refuses a missing token with 401). The Linux binaries were built
  and measured, not run.

Commands: `bench/size/native/measure.sh OUTDIR` prints
`<program> <build> <bytes> <stripped> <dSYM bytes>` per build (XO, GO,
CARGO override the tools); `breakdown.py BINARY [--modules]` splits an
Xo darwin/arm64 binary's code by module; `rodata.py PROG.s` splits the
read only data of the assembly that `xo build --backend=arm64
--targets=linux/arm64` writes. Raw output: one run per platform (sizes
are deterministic for a given toolchain).

## Results

Sizes in KiB, as "file / after `strip`"; on darwin the separate dSYM
bundle, when the build makes one, is "(+N)". Rust's release profile and
Go make no dSYM.

### darwin/arm64

| Build | hello | https_get | json_server | notes |
|---|---:|---:|---:|---:|
| Xo arm64 | 685 / 631 (+592) | 2,526 / 2,390 (+1,580) | 2,803 / 2,664 (+1,640) | 3,259 / 3,099 (+1,792) |
| Xo arm64 `-s` | 685 / 631 | 2,526 / 2,390 | 2,803 / 2,664 | 3,259 / 3,099 |
| Xo LLVM | 441 / 422 (+276) | 2,388 / 2,311 (+1,244) | 2,735 / 2,649 (+1,392) | 3,212 / 3,101 (+1,648) |
| Xo LLVM `-s` | 430 / 422 | 2,346 / 2,311 | 2,689 / 2,649 | 3,153 / 3,101 |
| Go | 2,434 / 2,329 | 7,865 / 7,448 | 8,148 / 7,709 | 8,476 / 8,021 |
| Go `-ldflags="-s -w"` | 1,619 / 1,619 | 5,384 / 5,384 | 5,564 / 5,564 | 5,812 / 5,812 |
| Rust release | 425 / 335 | 2,651 / 2,127 | 978 / 755 | 1,141 / 889 |
| Rust release, `strip = true` | 335 / 335 | 2,127 / 2,127 | 755 / 755 | 889 / 889 |

### linux/arm64 (Docker)

ELF keeps its DWARF in the file, so "file" includes debug info and there
is no separate bundle.

| Build | hello | https_get | json_server | notes |
|---|---:|---:|---:|---:|
| Xo arm64 | 1,286 / 645 | 4,064 / 2,373 | 4,387 / 2,629 | 5,006 / 3,077 |
| Xo arm64 `-s` | 708 / 645 | 2,508 / 2,373 | 2,768 / 2,629 | 3,233 / 3,077 |
| Xo LLVM | 690 / 450 | 3,708 / 2,562 | 4,078 / 2,818 | 4,907 / 3,394 |
| Xo LLVM `-s` | 477 / 450 | 2,663 / 2,562 | 2,929 / 2,818 | 3,532 / 3,394 |
| Go | 2,275 / 1,515 | 7,787 / 5,350 | 7,992 / 5,484 | 8,340 / 5,754 |
| Go `-ldflags="-s -w"` | 1,536 / 1,515 | 5,376 / 5,350 | 5,504 / 5,484 | 5,760 / 5,755 |
| Rust release | 458 / 325 | 3,039 / 2,181 | 1,111 / 773 | 1,303 / 901 |
| Rust release, `strip = true` | 325 / 325 | 2,181 / 2,181 | 773 / 773 | 901 / 901 |

### Reading the tables (darwin, stripped)

- Against Go `-s -w`, Xo arm64 is 0.39x (hello), 0.44x (https_get),
  0.48x (json_server), 0.53x (notes).
- Against Rust stripped, Xo arm64 is 1.9x (hello), 1.12x (https_get,
  both carrying a TLS client), 3.5x (json_server), 3.5x (notes). The
  Rust servers use tiny_http with no TLS; the Xo servers carry a full TLS
  1.2 and 1.3 server and HTTP/2 whether they serve TLS or not (below).
- The two Xo backends end within 3.5 percent of each other on the three
  network programs once stripped (LLVM 3.3 percent smaller on
  https_get, even on notes); LLVM is 33 percent smaller on hello.
- `-s` does nothing for an arm64 build on darwin (685 and 685 KiB; the
  macho linker writes the symbol table either way). On Linux it removes
  the DWARF (1,286 to 708 KiB on hello) but not the symbol table (708
  against 645 stripped). LLVM `-s` sits between the two as well.
- The dSYM of an Xo build is about half to nine tenths of the binary.

## Where an Xo arm64 binary's bytes go (darwin, notes and friends)

Code (`__text`) attributed per symbol by `breakdown.py`, in KiB, then the
other sections whole. "generated" is the per type code the compiler
emits (drops, shares, hashes, map and JSON instances); "runtime" is the
rest of std/native/llrt (scheduler, allocator, poller, unwinder, Unicode,
numbers, entry). The modules in each group are in breakdown.py.

| Part | hello | https_get | json_server | notes |
|---|---:|---:|---:|---:|
| code: the program | 0 | 3 | 8 | 43 |
| code: generated | 13 | 54 | 76 | 99 |
| code: TLS and crypto | 0 | 884 | 1,105 | 1,219 |
| code: HTTP and net | 7 | 320 | 413 | 420 |
| code: JSON, SQL, log, time | 0 | 55 | 25 | 152 |
| code: prelude | 4 | 10 | 12 | 17 |
| code: profiler and its line tables | 158 | 159 | 159 | 159 |
| code: test world (FakeClock, FakeNet) | 6 | 21 | 16 | 16 |
| code: other OSes (linux, windows on darwin) | 13 | 22 | 17 | 17 |
| code: runtime | 252 | 301 | 291 | 307 |
| `__const` | 53 | 234 | 226 | 293 |
| `__cstring` | 44 | 132 | 124 | 137 |
| `__eh_frame` | 25 | 69 | 70 | 80 |
| `__gcc_except_tab` | 6 | 38 | 38 | 45 |
| `__unwind_info` | 8 | 19 | 19 | 21 |
| `__data` | 2 | 2 | 2 | 2 |
| sections total | 593 | 2,325 | 2,603 | 3,027 |

Read only data (`rodata.py`, from the linux/arm64 assembly, in bytes):

| Kind | hello | https_get | json_server | notes |
|---|---:|---:|---:|---:|
| string literals of 256 bytes or more (crypto, HPACK, Unicode tables) | 22,840 | 144,444 | 147,464 | 205,186 |
| fault locations (`file.xo:line:col` per check that can fault) | 43,597 | 131,817 | 124,331 | 137,749 |
| other string literals | 35,297 | 98,640 | 85,366 | 94,900 |

The largest modules of json_server's code (KiB): tlssrv 372, tls13srv
172, tls12srv 139, x509 120, h2srv 86, prof 81, dwline 73, http1 69,
dns 49, nistec 41, unwind 39, hserve 37. LLVM builds keep 940 symbols
(locals are gone after inlining), so this split is for arm64 only.

### What that says

1. **A plain HTTP server carries the TLS server.** json_server serves
   only plain HTTP and has 1,105 KiB of TLS and crypto code (52 percent
   of its 2,122 KiB of code) plus the HTTP/2 server (h2srv, hpack): hconn.xo picks
   TLS per connection at run time (`HC_TLS`), so the linker keeps the
   whole server stack. The same holds for every `http.server`.
2. **The profiler is in every binary.** prof, dwline, and heapprof are
   158 KiB of hello's 593 KiB (27 percent) though nothing sets XO_PROF
   (decisions 0238, 0239).
3. **Fault locations are 4 to 6 percent.** 124 to 138 KB of
   `file:line:col` strings in the network programs (3,964 in
   json_server), stored whole, path and all.
4. **Code for other platforms and for tests.** A darwin binary carries
   linux.xo and windows.xo (13 to 22 KiB) and the test world's FakeClock
   and FakeNet (6 to 21 KiB).
5. **The program itself is small.** notes' own code is 43 KiB; its
   std/sql dependency is 125 KiB.

## Found along the way

The native HTTP client sends `Accept-Encoding: gzip` and returns the
compressed body undecoded (example.com: 400 gzip bytes on arm64 and
LLVM, 577 HTML bytes on the Go backend, Go, Rust, and curl); gotcha
[native-http-client-gzip-body.md](../../knowledgebase/gotchas/native-http-client-gzip-body.md).
It does not change the sizes above.
