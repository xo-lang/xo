# 0616: The testing evidence for a Proven std module

Status: Accepted (owner, 2026-11-12). Proposed by Xo Lang 3, 2026-11-12, at the lead's request; the owner set std maturity to Proven as the std track on 2026-11-12).

## Context

Decision 0461 grades each std module on three axes (features, testing,
performance) and makes its level the weakest of the three. The testing
axis reaches Proven with "`--race` and load stress clean, long fuzz runs,
security reviewed where it applies (crypto, tls, http, x509)". As of
2026-11-12 (main 26534b74e, website/scripts/gen-stdcov.py) the page reads
0 Proven, 25 Solid and 62 Early of 87 modules. Testing has no Proven module:
61 Solid, 26 Early. Performance has 17 Proven and features 37.

Three things block a Proven testing mark today:

1. Nothing defines the evidence. "Long fuzz runs" has no duration or case
   count, "load stress clean" names no load, and no file records a result.
2. The generator cannot read evidence. The testing axis is a table typed
   into gen-stdcov.py (the FUZZ dict of harness paths and descriptions,
   website repository), so a mark moves by editing the generator, which
   decision 0461 rules out ("moving a mark means adding evidence, not
   editing the page").
3. The tools exist one by one but not as a run. The harnesses are the
   differential fuzz tests of internal/stdtest (18 files: json, xml, csv,
   toml, png, jpeg, cbor, compress, archive, mime, mail, http head,
   response and URL, protobuf, yaml, time), seeded generators compared
   against Go's package, with `XO_<NAME>_SEED` and `XO_<NAME>_N` for longer
   runs. `--race` is native ThreadSanitizer (decision 0273). The offload
   hosts (infrastructure/offload.md) can run hours of either. No security
   review of a std module has been written.

## Decision

### 1. Evidence files

Each module's testing evidence is committed under `std/evidence/MODULE/`,
one file per kind, named by kind: `race.md`, `stress.md`, `fuzz.md`,
`review.md`. Each starts with a header the generator parses, one
`key: value` a line, then free text:

```
kind: fuzz
module: json
result: pass
commit: 26534b74e            # the main commit the run built
source: 3f2a...              # the module's source hash at that commit (section 3)
date: 2026-11-12
host: linux/arm64 host 2
command: scripts/stdproven/run.sh json fuzz
measure: 1000000 cases, 8.0 h, 2 backends (arm64, llvm), 0 mismatches
```

A failing run is not committed as evidence: it becomes a fix with a
regression case, then a new run.

### 2. What each kind requires

- **race**: the module's own tests (`xo test --std-module=M`) and its corpus
  test (`TestStdCorpora/arm64/M`) under `--race` on linux/arm64, plus the
  stress program of the next item under `--race` for 10 minutes. Pass: no
  data race report, no fault. Modules with no concurrency of their own
  (pure parsers, codecs) still run it: their callers share values across
  tasks.
- **stress**: a program per module under `std/evidence/MODULE/stress/`
  (its main.xo in the module's evidence directory, built by the runner)
  that runs the module's main operations from 64 tasks for 30 minutes on
  host 2. Each task checks every result against a precomputed answer. It
  samples resident memory every 10 s. Pass: no mismatch, no fault, and
  resident memory after the first 5 minutes grows by under 10% (no leak
  under load). For servers (http, tls, websocket, grpc, smtp): 64
  concurrent clients against the module's server over loopback, with
  hostile clients mixed in (slow readers, half sent heads, resets).
- **fuzz** ("long"): every fuzz harness that names the module runs on
  its native backend and LLVM for at least 1,000,000 generated cases per
  backend, or 8 hours, whichever ends first (the hours bound the slow
  harnesses, such as the image decoders). Pass: 0 mismatches
  against Go and 0 faults or hangs (a case over 10 s is a hang). Each case
  that found a defect during the run is shrunk and appended to the module's
  committed corpus (`testdata/corpus/MODULEfuzz/`) with the fix. The run
  records the seeds it used, so a later run starts past them. A module
  that parses untrusted input and has no harness writes one first: that
  is already its Solid requirement (0461).
- **review** (crypto, tls, http, x509, and any module 0461 lists as
  parsing untrusted input that opts in): an agent-written security review
  against `knowledgebase/infrastructure/std-security-checklist.md`, a
  checklist this decision adds:
  1. Go's CVE history for the package (the Go vulnerability database
     entries for its import path): for each, whether Xo's code shares the
     defect, with a test that reproduces the CVE's input and shows Xo's
     answer.
  2. Limits on hostile input: every length, count, depth and time a peer
     controls (header bytes, header count, nesting depth, decompression
     ratio, handshake messages, certificate chain length), the limit
     applied, and a test at the limit and one past it.
  3. Constant time (crypto, tls): the operations on secrets (MAC compare,
     padding checks, scalar multiplication, RSA private operations, AEAD
     tag checks), how each avoids data dependent branches and memory
     indexes, verified by reading the code and by a timing test
     (dudect-style: two input classes, a t-test over at least 10^6
     measurements per operation, |t| < 4.5).
  4. Fault behaviour: no input a peer sends makes the module fault (a
     fault is a crash of the server); each fault site reachable from
     input is listed with the error it returns instead.
  5. Defaults: insecure options off by default (TLS versions, cipher
     suites, redirects across schemes, cookie scope).
  A second agent reads the review against the code and signs its last
  section. Findings become fixes before the review is committed. A review
  goes stale with the module (source hash) and is redone when its module
  changes in a security relevant file.

### 3. Staleness

A module's source hash is the SHA-256 the runner computes over the paths
and contents, sorted by path, of the module's std/MODULE.xo,
std/native/MODULE.xo (and its std/native/llrt files) and the same files of
every std module it imports, directly or not. Evidence counts while that
hash matches its header's `source`, or
for 90 days and one minor release after the run, whichever is later. The
page shows the evidence's commit and date. Stale evidence moves the mark
back to Solid until a new run lands. A reviewer can run any kind again
from its header's command.

### 4. One runner, built once

`scripts/stdproven/run.sh MODULE KIND` runs one kind on the offload host
the kind needs (race, stress and fuzz on host 2, linux/arm64; timing tests
on host 1). It writes the evidence file with its header into the worktree,
for review and commit. `scripts/stdproven/run.sh MODULE all` runs race,
stress and fuzz in sequence. The fuzz kind reads each harness's existing
`XO_<NAME>_SEED` and `XO_<NAME>_N` variables. Harnesses gain an
`XO_<NAME>_HOURS` limit and a shrink-and-append step for failures, in one
shared helper of internal/stdtest.

### 5. The page reads the files

The coverage generator moves into this repository as
`scripts/stdcov/main.go` (Go, like stdaudit). The website keeps a thin
wrapper that calls it. It reads the testing axis from committed files
only: Solid from the corpus and the fuzz harness (as now, from paths, not
a typed table), Proven from `std/evidence/MODULE/*.md` with `result: pass`
and not stale, with each kind's link. The FUZZ table of gen-stdcov.py goes
away.

### 6. Order

Build the runner, the checklist and the generator change once. Then run
module by module in the owner's order: json, http, time, tls, x509,
crypto; within it, a module that misses only the testing axis first.

## Consequences

- A Proven testing mark has a committed, rerunnable record, and the page
  stops depending on hand-typed tables.
- Each module costs at most about 10 offload host hours (fuzz capped at
  8 h, 30 min stress, the race runs, building); the first runs measure
  the real figure.
- Security reviews add documents that age. The source hash makes the age
  visible instead of silent.
- Stress programs are new code per module, about 100 to 300 lines each.
  They also serve as regression load tests.

## Alternatives

- **Coverage guided fuzzing** (libFuzzer over the native build, Go's
  `go test -fuzz` over Go's package as the oracle): finds deeper paths
  than seeded generators. Not chosen first: the differential harnesses
  already compare every case with Go and need no instrumented build. It
  can be added later as a fifth evidence kind without changing the format.
- **Time based "long" only** (8 hours for everything): a fast harness
  would run far past the point of new findings. A count bound with a
  time cap spends host time where it buys cases. Each harness's rate is
  measured in the runner's first run and recorded in its evidence header.
- **Evidence in bench/results**: results files hold measurements;
  evidence is a per-module, per-kind current state that is replaced, not
  appended. A directory per module keeps the latest one obvious.
- **Human security review only**: none is scheduled. The agent-written
  review against a fixed checklist, with a second agent signing it, gives
  a record now. An outside audit can replace it later under the same
  file name.
