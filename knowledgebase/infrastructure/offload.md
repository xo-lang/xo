# Offload host

A Linux arm64 machine on the owner's network that runs part of the lead's
full tier, so the Mac's heavy slots serve gates and measurements (owner,
2026-10-29: the offload container instead of routine CI). The repository
names no address or password (decision 0249): they live in
`~/xo-lead/offload.env` (mode 600) as `OFF_HOST`, `OFF_PORT`, `OFF_USER`,
`OFF_PW` and `OFF_SEED`.

Last verified: 2026-10-30

Any test, build or measurement may run on either host without asking
(owner, 2026-10-29: they exist to take load off the Mac); each runs one
job at a time. They replace the EC2 CI (decision 0423).

## What runs there

`scripts/lead/tier.sh` sends the tier's commit as a git bundle to the
second arm64 host, or the first when that one does not answer (since
2026-11-01: a step on the first host holds back every `--ab`). The bundle
has the commits since `refs/offload-arm64b/sent` (`refs/offload/sent` on
the first host), all of them the first time; the host
has no access to the private repository, and the public one's hashes
differ, since publish.sh rewrites its history) and runs
`scripts/lead/offload-tier.sh` there in the background, beside the Mac's
steps, logging to `~/xo-lead/agents/lead-tierN-off-cmd.log`:

- moved off the Mac (they fail the tier): `gen` (the Go backend), `fuzz`,
  internal/selfhost's merge tier (the wasm goldens);
- Linux coverage the Mac cannot give (reported as `linux NAME: ...`, not
  failing the tier until their known failures are fixed): `llvm`, `cmd`.

A step's exit 5 means only a `linux` step failed: the host's last line
then reads `offload tier N: ok, linux steps FAILED: NAME`. `gate.sh`'s whole
output for a failed step is in `/tmp/xo-gate-tierN/NAME.txt` on that host.
While a tier holds the Mac, `hourly.sh` runs main's fixed point on the
second host (offload-tier.sh's `fp` step, logged to `hourly.log`), then its
usual check after the tier. Last verified: 2026-11-01.

When no host answers, tier.sh runs every step on the Mac as
before. The Mac keeps `short`, `arm64`, `llvm`, `cmd` and `corpora-all`
(darwin coverage), the newest Go and sepcomp Docker steps.

## Tools

- `scripts/lead/offload.sh SCRIPT|- [ARG...]` runs a bash script on the
  host as user `xo` under `flock /work/offload.lock` (one job at a time)
  and exits with its status; `--put LOCAL REMOTE` copies a file (mode
  644, owned by the host's ssh user: copy it before chmod in a job). REMOTE's
  directory must exist: a missing one fails with scp's message and a hint
  (scripts/lead/offload_test.sh; Last verified: 2026-11-02). It needs
  no heavy.sh slot (only ssh runs on the Mac). Exit 3: no offload.env;
  exit 4: no answer. On the arm64 host a job is stopped after
  `OFF_MAX_MIN` minutes holding the lock (default 180, exit 124, with a
  message; tier.sh's job 300), so a hung run cannot hold the one slot
  while the queue grows (lead, 2026-10-30: 9 jobs queued behind one run).
  Before each arm64 job, when /work has under 30 GB free, offload.sh
  deletes xo cache files idle over 6 hours (1 hour under 15 GB free;
  gocache, backend, testcc, testbin, build under ~xo/.cache/xo), then
  the empty directories idle as long (2026-11-03: with no age it removed
  running builds' fresh work directories, gotchas/empty-dir-prune-removes-live-build-dir.md),
  and prints the free space after (lead, 2026-10-30: the cache regrew
  to 55 to 61 GB and filled /work three times that day).
  The same pass removes whole checkout/ entries (xo builds, about
  88 MB each) when none of their files was read or written for 12 hours
  (owner, 2026-11-07: about 90 of them held 8 GB on host 1 while /work
  fell to 5.7 GB free).
  It also removes `/work/in/*.bundle` files neither read nor written for
  6 hours (lead, 2026-11-07: about 140 bundles of about 100 MB each held
  14 GB on host 2 at 12 GB free).
  The arm64 container runs 2 jobs at once (OFF_SLOTS=2 in offload.env;
  owner, 2026-10-30): slot 1 is `/work/offload.lock`, slot 2
  `/work/offload.lock.2`. A job that measures time or memory sets
  `OFF_EXCLUSIVE=1` and waits for both slots, so it runs alone
  (ab-host.sh sets it; its remote half then waits, at most
  AB_QUIET_MAX seconds (300), for the 1 minute load average to fall below
  AB_QUIET_LOAD (1), since an exclusive lease that starts as a long
  benchmark ends inherits its load: 8.03 on 12 CPUs on 2026-11-02, Last
  verified: 2026-11-02). It takes both at once or neither, so a plain job
  never holds a slot while it waits. A job that needs much of the host's memory
  (an LLVM golden run with clang, a big Go test) sets `OFF_MEM=1` (heavyd
  `lease -mem`; Xo Lang 2, 2026-11-07, after host 2 OOM killed clang in
  LLVM goldens and a Go test while several leases ran at once): it starts
  only when no other lease runs on its lane, and no lease starts beside it
  while it runs, as an exclusive one, but it does not wait for the host's
  no-lane runs (soaks), and the remote side takes one slot lock, not all;
  `heavyd status` shows it as `memory`. Gate aging (lead, 2026-11-08
  bottleneck review: gates waited 36 to 60+ minutes behind exclusive runs
  on host 2): once a memory lease (offload-gate.sh's gates) has waited
  HEAVYD_GATE_AGE_MIN (20; 0 off), no waiting exclusive lease starts on
  that lane before it, and the exclusive one says why in `heavyd status`;
  a running exclusive lease is not stopped (heavyd lanes.go GateAge,
  TestLaneDecide; Last verified: 2026-11-08). After heavyd starts (a restart, an
  install), it grants no lane lease for 15 s (AdoptGrace), so the runs
  still going from before re-adopt their slots first (lead, 2026-11-07: a
  new lease took pass2's slot 1 in the same second an adopted run came
  back to it). A lane only: the amd64 and win
  lanes share one machine, so OFF_MEM=1 there does not hold the other
  side. Last verified: 2026-11-07. Jobs start in arrival order: each
  takes a ticket in `/work/offload.q`, and only the oldest live ticket may
  take a slot (one free slot for a plain job, every slot for an exclusive
  one), so a job behind an exclusive one waits for it (2026-10-30: nine
  jobs waited 33 to 92 minutes while slots went out of order and a
  waiting timing job held one idle). One exception, backfill (owner,
  2026-10-30): while the head ticket is an exclusive job and a long job
  (OFF_MAX_MIN over 180, the full tier's 300, or at 180 and already 20
  minutes in, since 2026-10-31) holds a slot, the oldest
  plain job takes the free slot, since the exclusive one cannot start
  before the long job ends (tier 31 held a slot about two hours while
  twelve jobs waited behind an exclusive --ab). heavyd's lanes backfill
  only a job that is not long itself (lead, 2026-11-03, writer
  preference): a backfilled long job kept backfill open after the jobs
  the exclusive one waited for had ended, so a chain of them could hold
  it off for good (scripts/lead/heavyd TestLaneExclusiveNotStarved;
  offload.sh's own ticket queue, used with OFF_NO_LANE=1, still
  backfills any plain job; Last verified: 2026-11-03). At most 2 arm64 jobs
  per agent, waiting or running: offload.sh counts the jobs of its
  Claude session and refuses a third with exit 5 (OFF_MAX_QUEUED,
  0 turns it off; lead, 2026-10-31: one agent had five --ab runs
  queued). A job whose process tree uses no CPU and starts no process
  for OFF_IDLE_MIN minutes (default 15, 0 off) is stopped with exit 125
  (lead, 2026-10-31: a bisect hung on stdin for 73 minutes). Only arm64 jobs count (each records its pid in
  ~/xo-lead/locks/offq; since 2026-10-31 amd64 jobs no longer use up
  the cap). A job whose ssh connection drops (its Mac side stopped) ends
  on the host too, within about 15 s, and its ticket goes with it.
  Stopping a run (2026-11-02, after a stopped run went on on the amd64
  host and its cleanup deleted the next run's directories): each run has
  an id, and the host records the run's shell and its job (started with
  setsid) in /work/offload.runs/ID (ssh host) or ~/xo-ci/runs/ID (WSL and
  Git Bash). A TERM, INT or HUP to offload.sh (a TaskStop of the caller)
  stops the local ssh and then, through a second ssh, every process of the
  run on the host: the recorded PIDs and their descendants by /proc parent
  links, the job's process group, and on Linux the job's session, which
  also holds `timeout`'s own groups and orphans; TERM, then KILL after
  3 s; by PID and group, never by name. That second ssh runs in a
  process group of its own, since a TaskStop kills the caller's whole
  group a moment after its TERM. Output now streams line by line (perl,
  not tr and grep, which held it in a pipe buffer until the job ended).
  Tested 2026-11-02 with a TaskStop of a sleeping job on the first arm64
  host, the amd64 WSL host and Windows (nothing left, records removed),
  and in scripts/lead/offload_test.sh. A kill -9 of offload.sh, which
  runs no trap, ends its local ssh within 5 s (a watcher), and the ssh
  host's wrapper then stops the job as above; the WSL and Windows kinds
  have no such wrapper, so stop their callers with TERM. Last verified:
  2026-11-02.
  Since 2026-10-31 (decision 0451 stage 2) the queue, the exclusive and
  backfill rules and the cap live in heavyd's lanes: offload.sh waits in
  the lane for its host (`arm64` for the ssh host, `amd64` for the WSL
  one, or OFF_LANE in the host file) with `heavyd lease`, then runs the
  job as before; `~/xo-lead/agents/heavy.sh --status` lists every lane,
  who holds it and why each job waits. The cap is the lane's
  (`~/xo-lead/heavyd-lanes.conf`, `NAME SLOTS CAP`; 2 per agent on
  arm64), still exit 5. The host's own queue stays as a safety net and
  for jobs sent while no daemon answers (offload.sh then queues there as
  before); OFF_NO_LANE=1 skips the lane. The arm64 host runs 3 jobs at
  once since 2026-10-31 (owner: OFF_SLOTS=3 in offload.env, `arm64 3 2
  offload.env` in heavyd-lanes.conf; an exclusive run still takes the
  whole host). OFF_PRIORITY=1 puts a job ahead of every waiting job
  without it, exclusive ones included (it never stops a running one):
  for a run that confirms a fix to main or unblocks another agent, not
  for measurements (owner, 2026-10-31). A job that names no host file
  is routed to the first free lane of its kind (`arm64*`, `amd64*`), so
  adding a host is its env file plus one heavyd-lanes.conf line
  (`arm64-2 2 2 offload-2.env`); a job that names one (OFFLOAD_ENV, as
  ab-host.sh, tier.sh and amd64-run.sh do around their `--put`) stays on
  that host. Last verified: 2026-10-31.
- `bench/perf/jitter/alloc-split.sh MODE SPECS NAME ROUNDS` (Lang 3, 2026-11-02, the Lead's bottleneck
  review: host 1's exclusive lease held the builds of every revision): the builds of a /alloc comparison run on
  the second host (`alloc-build-remote.sh`, lane pass2, not exclusive: the revisions' bundle, one xobuild and
  one jitter server per spec, the base server from `$OFF_SEED`, the loadgen; MODE `last` builds every server
  from the last revision's `bench/perf/jitter/xo`, `own` from each revision's own tree), the tar of binaries
  (`bin/`, `env-pK`, `revs.txt`) goes second host, Mac (`offload.sh --get`, 43ca0e3d7), first host `/work/in`,
  and `alloc-measure-remote.sh` holds the exclusive lease for the quiet wait and the rounds only (servers on
  CPUs 0-5, loadgen 6-11). Default build place since 2026-11-02 (the Lead: a build waited 36 min for pass2 while
  host 1 sat idle): the Mac, `alloc-build-mac.sh` as a `HEAVY_CLASS=gate` job cross-building linux/arm64 with the
  darwin v0.35.0 xo (`XO_MAC_SEED`, default `~/xo-seed-0.35.0-21620368/xo/bin/xo`); `ALLOC_BUILD=host2` keeps the
  second host. SPECS are `REV[+VAR=VALUE...]`: the VARs apply to that server's xo build and run
  (`XO_ARM64_PAD=2`, `XO_DATA_SHIFT=64`). Measured on a two revision test: 33 s of build work off host 1 (warm
  caches); a cold compiler build per revision is minutes. Both hosts' `$OFF_SEED` must be the same version
  (v0.35.0 on 2026-11-02), since the base ratio is against it. Last verified: 2026-11-02.
  Since 2026-11-03 (Lead) every spec and the base release are built at each pad of `ALLOC_PADS` (default
  `0 1 2 3`) and `pads.py` judges each against the base by its median over pads (bench/perf/jitter/README.md).
- `scripts/lead/ab-host.sh BASE NEW NAME [--rounds=N] [--warmup=N] --
  self|self=a|edit PROG [FLAG...]|PROG [ARG...]` (`self=a`: BASE's
  compiler/xo as the self build's program, for a NEW whose compiler calls
  a std function BASE's std lacks; 2026-10-30) (lead, 2026-10-30: a wall `--ab` whose claim is not
  darwin specific runs here, so the Mac's measure windows serve only
  darwin claims and footprints): `xo bench --ab` of two committed
  revisions. One bundle of the commits the host lacks (since
  `refs/offload/sent`, which tier.sh shares and which a run advances
  only to a revision on main; `refs/offload-amd64/sent`
  with `OFFLOAD_ENV=offload-amd64.env`), then `ab-host-remote.sh` there:
  two detached worktrees under the priv lock, xo built at each with
  `OFF_SEED` (the host's cache when it has the build), caches off, and
  the self build (`self`: each side's xo builds NEW's compiler/xo with
  its own std, `--keep-loaded`), the warm rebuild of a program (`edit
  PROG [FLAG...]`: the front end and std half from warm caches in a
  private HOME and a backend cache per side, bin and run caches off so
  every round links, each side filling the std cache and building twice
  first, one copy of the program for both sides since its path lands in
  the debug strings; `FLAG...` are `xo build` flags, a `--targets=` among
  them the std cache's target too and the sides then build separately
  with `XO_SEPCOMP=1` from stdcache's store and facts (a cross build
  gets none by default), for a cross link's claim; 5 warmups
  by default) or a program directory built by each side and run
  with `ARG...`; 3 rounds by default (decision 0340). The table
  streams back; the host keeps only the logs in `abNAME.out` (the two
  worktrees, the built xo and binaries, and the edit loop's caches go
  when the run ends, however it ends: 1.1 GB per run filled the disk,
  2026-10-30). No heavy.sh slot.
- `scripts/lead/offload-tier.sh SHA N SEED [BUNDLE]` (run through
  offload.sh): fetches the bundle into `/work/priv`, checks SHA out into
  `/work/tierN`, builds xo (`go run ./scripts/xobuild` with `XO_SEED`, or
  the host's cached xo of an ancestor commit; never the seed chain), then
  the steps above; logs in `/work/tierN.*.txt` (the checkout itself
  goes when the run ends).

## The host's setup

A plain `golang:1.26.1-trixie` container (Go 1.26.1, Debian trixie, 12
CPUs, 39 GB) with ssh on a mapped port and password login, set up by hand
on 2026-10-29. Recreated by the owner on 2026-10-30 after its ext4 /work
went emergency read only (at 60% of 453 GB, a filesystem error, not a
full disk): now with `--init` (docker-init as PID 1 reaps orphans; the old
`sleep infinity` PID 1 had piled up 42,652 zombies) and an 89 GB disk, so
clean up /work after each job. Recreated again the same day (owner,
2026-10-30 afternoon) with `--security-opt seccomp=unconfined` for perf;
/work survived (a volume), the system did not, so the lead reran the setup
(perf, tini, the second slot's lock, /work/in, the public 0.35.0 seed as
OFF_SEED). perf runs, but the VM exposes no hardware counters (no
armv8_pmuv3 under /sys/bus/event_source/devices; instructions and cycles
are `<not supported>`), so instruction sweeps (bench/perf/sweep.py) run on
the Mac (Last verified: 2026-10-30). The lead reinstalled the list below
(openssh-server first: `apt-get install openssh-server`, `mkdir -p
/run/sshd`, PermitRootLogin and PasswordAuthentication in
/etc/ssh/sshd_config.d/, `/usr/sbin/sshd`, which a container restart does
not restart; the Mac's known_hosts entry changes with the host key):

- upstream LLVM 23.1.3 in `/opt/llvm` (the CI runners' version and
  SHA-256), `clang.cfg` with `-fuse-ld=lld` and `-mcpu=native`;
  `/etc/profile.d/xo.sh` puts `/opt/llvm/bin` and Go on PATH and sets
  `XO_CC=xo-cc` (`/usr/local/bin/xo-cc`, clang with `-mcpu=native`);
- nodejs, make, gcc, python3, time, flock;
- the user `xo` (tests run as non-root: as root, the permission goldens
  such as fs_ops_edges print other output);
- the seed: the 0.37.0 linux-arm64 seed in
  `/work/seed/xo-0.37.0-linux-arm64` (both arm64 hosts; last verified:
  2026-11-03); after a minor version bump that seed cannot build,
  unpack the new seed there and change `OFF_SEED`. The base of a
  comparison does not follow the seed (2026-11-03): `alloc-split.sh`'s
  base server is built by `ALLOC_BASE_XO` (the Mac build: a darwin xo,
  default `XO_MAC_SEED`, v0.35.0; `ALLOC_BUILD=host2`: required) and
  release-vs-go.sh's http section by `HTTP_BASE_XO` (required), each the
  release compared against.
- `/work/azure1-specs` (the second host; owner Xo Azure 1, 2026-11-07):
  the Azure port's spec cache, the azure repo's `codegen/.specs` (a
  pinned commit's folders and its `.store` partial clone, about 1 GB),
  kept between the Azure differential batches. Each batch job sends only
  the files changed since its last run and points its run's
  `codegen/.specs` at it; the job's trap removes its own run directory
  and leaves this one. Delete it to free the space; the next batch sends
  it whole again.

### WebAssembly tools (owner-approved 2026-10-30, decisions 0405 and 0447)

Installed under /work/tools (it survives container recreates), pinned:
wasmtime 50.0.1, wasm-tools 1.263.0, TinyGo 0.44.0 (go1.26.1, LLVM 22.1.4),
Rust 1.99.1 (rustup, minimal profile) with the wasm32-wasip2 target. A job
uses them with

```
export RUSTUP_HOME=/work/tools/rustup CARGO_HOME=/work/tools/cargo PATH=/work/tools/bin:$PATH
```

The Rust homes belong to the job user xo, so cargo can fetch crates; the
rest is root-owned and read-only to jobs. About 2.0 GB. To upgrade, unpack
the new release beside the old one and move the symlink in /work/tools/bin.
Last verified: 2026-10-30.

## Findings (2026-10-29)

- internal/llvm takes 30 minutes there against 46 to 56 on the loaded Mac.
- clang's generic armv8 target rejects `eor3` (SHA-3) and, before
  1b0a7dcb9, `crc32x`: `-mcpu=native` in clang.cfg.
- Ten internal/llvm tests fail only on Linux arm64 (the list in the lead's
  notes, `offload/linux-arm64-llvm-fails-2026-10-29.md`), assigned to
  Xo Lang 3.

## The second linux/arm64 host

The owner's second machine (2026-11-01): the same container setup on an M5
Pro Mac, at a private address in `~/xo-lead/offload-arm64b.env` (18 CPUs, 23
GB VM, 3 slots since 2026-11-02, at most 2 per agent), lane `pass2`.
A full `internal/llvm` run there takes the whole host (`OFF_EXCLUSIVE=1`):
beside other jobs clang is `Killed` at the container's memory limit, which
reads as a failed golden (2026-11-02). The first host runs on an M4 Pro. The second
has `--cap-add=SYS_NICE` and `--ulimit rtprio=99`, which the first host lacks.
A job uses them with `OFF_RTPRIO=1` (offload.sh starts it as xo through
setpriv instead of `su - xo`): su's pam_limits sets rtprio to 0 whatever
limits.d says, so a plain job's `chrt -f` fails (Last verified: 2026-11-02). Same packages, LLVM, seed and WebAssembly tools as
above. Use it for pass/fail runs, with `OFFLOAD_ENV=offload-arm64b.env` on
both `--put` and the job, and a whole bundle the first time (a bare
`offload.sh` job stays on the first host, since agents' jobs lean on files
there). Its commits sent are in `refs/offload-arm64b/sent`. Memory-heavy runs
(the tier's selfhost and llvm steps) stay on the first host.

**`--ab` stays on the first host.** The lane's name is outside `arm64*` so
`ab-host.sh` does not pick it, because of these controls: identical compiler
and std on both sides (8fe1f704b and d1cd6520c, a script-only commit), self
build, 3 rounds, sides swapped between runs:

| host | wall B/A | CPU B/A | max RSS B/A |
|---|---|---|---|
| second (M5 Pro) | 0.957, 0.980, 1.040 | 0.930, 0.996, 1.048 | 0.999, 0.999, 0.994 |
| first (M4 Pro) | 1.004, 1.001 | 1.007, 1.002 | 1.007, 1.006 |

The verdict column called three of the second host's wall readings "better"
or "worse" and one of the first host's "worse" with identical code, so a
verdict at 3 rounds is not proof: read a small ratio against this table. On
the first host a wall ratio beyond about 1% is real; RSS read 0.6% higher on
side B whichever revision it was, so an RSS ratio within about 1% is not a
claim. The owner was using the second host's Mac during these runs (owner,
2026-11-01), which Linux's load guard cannot see, so that use is the likely
cause. Three more controls in the same conditions read wall 0.994, 0.987, 1.059.
Rerun the controls with that Mac idle before routing `--ab` there; timing runs
are only valid while it is idle. Last verified: 2026-11-01.

### linux/riscv64 under qemu-user (lead-approved 2026-11-08, decision 0571)

`scripts/riscv64-toolchain.sh` installs, under /work/riscv64 only (no
root, no system packages, no binfmt registration), the static qemu-user
for the host's architecture (trixie's qemu-user, which is static since
qemu 10; qemu-user-static is a transitional package there with no
binaries, and the script takes whichever holds qemu-riscv64) and a
riscv64 glibc sysroot: libc6,
libc6-dev, linux-libc-dev, libgcc-s1, libgcc-14-dev and libstdc++6 from
Debian trixie. Everything comes from deb.debian.org and is checked:
Release by its detached signature (Release.gpg) against the host's
debian-archive-keyring with sqv (the host has no gpgv: trixie's apt
verifies with Sequoia's sqv; the script takes gpgv where sqv is
missing), the Packages indexes by their SHA256 in Release, each .deb by
its SHA256 in the index; `dpkg-deb -x` unpacks them, so no maintainer script runs.
Run it through offload.sh with a lease, as job user xo:

```
OFFLOAD_ENV=offload-arm64b.env OFF_LANE=pass2 ~/xo-lead/agents/offload.sh scripts/riscv64-toolchain.sh /work/riscv64
```

A job then builds with `XO_LINUXSYSROOT=/work/riscv64/sysroot xo build
--backend=llvm --targets=linux/riscv64` and runs the binary by naming
qemu: `/work/riscv64/qemu/usr/bin/qemu-riscv64 BINARY` (the script
prints the path), with `-L /work/riscv64/sysroot` for a binary
that is not static. Rerunning the script rebuilds the directory from
scratch (it unpacks into /work/riscv64.new, then renames it).
sqv is the host's own /usr/bin/sqv, already installed before this setup
(a diagnostic run on 2026-11-08 found it there and no gpgv); the script
installs no system package and fetches no verifier, so the chain of trust
is the host's Debian install (its sqv and debian-archive-keyring).
Installed on host 2 on 2026-11-08: Debian 13.7's qemu-user
1:10.0.13+ds-0+deb13u1, libc6 2.41-12+deb13u4, linux-libc-dev
6.12.94-1, libgcc 14.2.0-19.
Last verified: 2026-11-08.

### Kept directories on host 2

- `/work/conf` (452 MB, rebuilt 2026-11-08; Last verified: 2026-11-08): Google's protobuf v29.3 `conformance_test_runner` and
  `protoc` (cmake build tree with abseil 20240116.0 and jsoncpp 1.9.6 as modules), plus `run/` with `set.bin` (FileDescriptorSet of
  test_messages_proto3.proto, test_messages_proto2.proto and editions/golden's two editions files, `protoc -I src/src -I src --include_imports`). It is a tool, not scratch:
  do not delete it in a cleanup. Rebuild (about 10 minutes, plain lease on host 2): unpack the Kitware cmake 3.31.6 linux-aarch64
  tarball into `/work/conf/cmake`, extract protobuf-29.3.tar.gz into `src` (strip 1), `git clone --depth 1` jsoncpp 1.9.6 and abseil-cpp
  20240116.0 into `src/third_party`, then `cmake -S src -B build -DCMAKE_BUILD_TYPE=Release -Dprotobuf_BUILD_CONFORMANCE=ON
  -Dprotobuf_BUILD_TESTS=OFF -Dprotobuf_ABSL_PROVIDER=module -Dprotobuf_JSONCPP_PROVIDER=module` and `cmake --build build --target
  conformance_test_runner protoc -j10`. Run: see test/protoconf/README.md; `offload.sh --put` files are root owned, so copy the
  testee before chmod +x.

## The linux/amd64 host

The owner's second machine (2026-10-29): a 16-core x86-64 box running
Windows with OpenSSH (password login, user `xo`, no administrator) and a
WSL2 Ubuntu 24.04 distribution for that user (28 CPUs, 62 GB from its
`.wslconfig`). Its values live in `~/xo-lead/offload-amd64.env` (mode
600) with `OFF_KIND=wsl` and `OFF_SEED` the WSL path of the 0.37.0
linux-amd64 seed (`~/xo-ci/seed-0.37.0/xo/bin/xo`; Windows itself uses
`~/xo-ci/seed-0.37.0/xo/bin/xo.exe`; last verified: 2026-11-03); WSL has clang 18, lld,
llvm, build-essential, expect and Go 1.26.0 in `~/go-sdk` (on PATH from
`~/.profile`). Windows' own shell is cmd.exe, so offload.sh sends a job
there as a file: scp (`-O`, the legacy protocol, which both hosts take)
to `%USERPROFILE%\xo-ci\off`, then `wsl -e bash -l` on it under
`flock ~/xo-ci/offload.lock`.

Not for HTTP throughput (Xo Lang 2, 2026-11-02): loopback HTTP inside WSL2
there runs about 20 times below the linux/arm64 hosts for Go and Xo alike
(bench/perf/jitter /alloc at 32 connections: Go 3,600 req/s at 870 to 920 us
CPU a request, Xo 8,100 to 8,600 req/s at 630 to 660 us), so the host's
network stack, not the program, sets the number
(gotchas/wsl2-loopback-http-throughput.md). Use it for amd64 correctness and
CPU-bound timings; HTTP throughput needs bare Linux. Last verified: 2026-11-02

- `scripts/lead/amd64-run.sh REV NAME [GOLDEN|all|sample|passes ...]` sends REV
  as a bundle (the commits since `refs/offload-amd64/sent`) and runs
  `scripts/lead/offload-amd64.sh` there: xo built natively at REV with the
  seed, then `scripts/linux-goldens.sh` with `--backend=amd64` and no C
  toolchain (as scripts/docker-goldens.sh). The default `sample` is
  scripts/docker-amd64-sample.txt. Logs stay in `~/xo-ci/runNAME.out` in
  WSL. Unlike the Docker run on the Mac, nothing is emulated (no Rosetta)
  and xo is the native backend's, not a Go backend cross build.
- `scripts/lead/amd64-run.sh REV NAME script PATH [ARG...]` (Xo Lang 2,
  2026-11-07, the lead's bottleneck review: host 1 had 44 lease waits of
  3 minutes or more in 3 hours while this host sat idle) runs any script
  of REV's tree instead of the goldens, for pass/fail jobs: the same
  bundle, priv lock and detached worktree, xo built with the host's seed,
  then `bash PATH ARG...` from the tree's top with `XO_CLI` set to that
  xo and its `bin` first on PATH. The output streams back and stays in
  `~/xo-ci/runNAME.out/script.txt`; the exit status is the script's; the
  tree is removed when the run ends, however it ends. Last verified:
  2026-11-07.
- `passes` (with or without goldens: `all passes`, which tier.sh runs
  every 8 hours) is the x86 counterpart of internal/arm64's
  TestGoldenPassesOff (2026-11-02): the pass hunt's goldens (`pass_*`,
  `slp_*`, `rc_empty_*`) built with `--backend=amd64` and each x86 only
  optional pass of compiler/arm64/passes.xo off alone (`XO_PASSES=-x86_...`,
  which also bypasses the backend and function caches), then with every
  optional pass off, each against its want.txt. A failure prints
  `FAIL NAME with XO_PASSES=-PASS` (or `BUILD ...`), which tier.sh's
  summary picks up; outputs in `~/xo-ci/runNAME.out/passes-PASS`.
- First runs (2026-10-29, main aa9c8794d): xo built natively in 222 s
  (1 s when the host's cache has it); the 48 goldens of the sample in
  33 s, against about 15 minutes in Docker on the Mac; `all` (433, less
  the `use_c_*` goldens, which need clang) in 183 s, which found
  http_wire and http_stream red on every backend.
- Windows itself (`~/xo-lead/offload-win.env`, `OFF_KIND=win`): Git Bash
  from PortableGit 2.56.0 in `%USERPROFILE%\tools\git` runs the job, Go
  1.26.0 in `tools\go`, the 0.34.0 windows-amd64 seed in
  `C:\Users\xo\xo-ci\seed\xo`. `OFFLOAD_ENV=offload-win.env
  scripts/lead/amd64-run.sh REV NAME [sample|all|GOLDEN...]` builds xo
  natively on Windows (229 s cold, 14 s cached) and runs the goldens
  there (windows/amd64; `want_windows.txt` where a golden has one,
  scripts/golden-run.sh); commits sent are `refs/offload-win/sent`. The
  priv repository sets `core.autocrlf false` (Git for Windows' default
  CRLF checkout broke go.mod). Git Bash has no flock: locks are
  directories (`~/xo-ci/offload.lockdir.N`, stale when its shell is gone).
- Cold Windows builds until the 0.37 seed (2026-11-03, decision 0318's
  2026-11-03 addendum): the 0.34.0 and 0.35.0 seeds cannot build main on
  Windows since the async stub change (their emitter defines no
  xo_async_stub, which xobuild's smoke build links). An xo of its first
  commit (built before the rebase that made it ffdfe7d76 on main) with its
  std is kept in
  `C:\Users\xo\xo-ci\seed\xo-7950290dbbb8` (`~/xo-ci/seed/xo-7950290dbbb8/bin/xo.exe`
  in Git Bash); pass it as the seed (`XO_SEED`, or a script's SEED
  argument) for a Windows build that has no cached ancestor build. Delete
  it once the 0.37 seed is on the host. Last verified: 2026-11-03.
- Windows privileges (owner, 2026-10-30): Developer Mode is on, so user
  xo creates symbolic links without elevation, and xo is an
  administrator with LocalAccountTokenFilterPolicy=1, so jobs on the box
  run elevated and `\\localhost\C$` (an admin share) works. With them
  fs_create_new, fs_files, fs_ops_edges, fs_walk_dir and fs_paths_os pass
  natively (main 4e39e0456); fs_symlink_info passes once a relative
  directory link is made a directory link (std/changes/prelude.md,
  2026-10-30). A golden that passes there may still fail for an
  unprivileged Windows user (no Developer Mode, no admin share).
  Last verified: 2026-10-30.
- The lead's full tier (tier.sh) runs the windows/amd64 sample here on
  every tier since 2026-10-30 (owner: no routine run covered native
  Windows), logging to `~/xo-lead/agents/lead-tierN-win-cmd.log`; a
  failure fails the tier.
- Slots: `OFF_SLOTS=2` in both of this machine's host files, so two jobs
  run at once per side (WSL and Windows), four in all; the arm64
  container stays at one (owner, 2026-10-29). The one repository's fetch
  and worktree change take turns (`~/xo-ci/priv.lockdir`).
- Goldens run `XO_GOLDEN_JOBS` at once (scripts/linux-goldens.sh; the
  offload job uses half the host's CPUs): all 433 on WSL in 20 s instead
  of 183. Under parallel load the host answers a dial to 192.0.2.1 with
  "Network is unreachable" instead of a timeout, so
  net_lookup_dial_timeout fails there until its output stops naming the
  error (Lang 2, 2026-10-30).

### FreeBSD VMs under QEMU (owner-approved 2026-11-08, decision 0573)

The amd64 host runs FreeBSD guests for what a cross build cannot prove
(the kernel's answers: syscalls, umtx, thr_new, kqueue, signals). The
owner set it up on 2026-11-08: the WSL user `xo` is in the `kvm` group
(`/dev/kvm` read-write from an offload job), and qemu-system-x86,
qemu-system-arm, qemu-efi-aarch64 and ovmf 8.2.2 are installed
(`/usr/share/ovmf/OVMF.fd`, `/usr/share/qemu-efi-aarch64/QEMU_EFI.fd`).
There is no dosfstools, mtools or ISO writer, so a guest's input is an
ISO 9660 image the Mac makes. Last verified: 2026-11-08

- Image: FreeBSD 14.5-RELEASE `BASIC-CLOUDINIT-ufs.qcow2` from
  download.freebsd.org VM-IMAGES (amd64, and arm64-aarch64 for a TCG
  guest), its sha256 checked against the directory's CHECKSUM.SHA256,
  cached unpacked in WSL's `/home/xo/xo-ci/l1fbvm/fbvm-cache` (WSL's own
  disk; the Windows home `/mnt/c/Users/xo` is slow for a VM disk). Each
  run boots a qcow2 overlay of it, removed after.
- Input: a NoCloud seed, an ISO 9660 image labeled `cidata`
  (`hdiutil makehybrid -iso -joliet -default-volume-name cidata` on the
  Mac) with `meta-data`, a `#!/bin/sh` `user-data` and the cross-built
  programs. FreeBSD's nuageinit finds it (`/dev/iso9660/CIDATA`, any
  case), runs the user-data late in boot, and has unmounted the seed by
  then, so the script mounts it again, copies the programs to /tmp, runs
  `run.sh`, prints between `XO-FB-BEGIN` and `XO-FB-END` on
  `/dev/console`, and powers off (`-nographic -no-reboot`: the serial
  console is QEMU's stdout).
- Run: `qemu-system-x86_64 -enable-kvm -cpu host -m 4096 -smp 4`, the
  overlay and the seed as virtio disks, user networking. A boot to the
  user-data takes about a minute under KVM. `truss` is in the base
  system, so a failing program is rerun under it in the same boot.
- Tools (Xo Lang 1, not yet in scripts/lead): `fbvm-seed.sh PAYDIR
  OUT.iso` on the Mac, `fbvm-run.sh ARCH SEED.iso [TIMEOUT_S]`, which
  puts the seed and `fbvm-remote.sh` up through the Windows home's
  `xo-ci/l1fbvm` (offload.sh `--put` paths are relative to the Windows
  home) and runs it under the host's offload lock, in
  `~/xo-lead/notes/lang1/`.
- First result (2026-11-08, xo of main 739e256b6): a freebsd/amd64 hello
  maps memory and sets its FS base, then calls syscall 293 (Linux's
  `pipe2`) from the poller's start (std/native/llrt poll.xo ev_init) and
  dies with SIGSYS.

## The tier's host steps (lead, 2026-11-01)

`tier.sh` runs `offload-tier.sh` as one `offload.sh` call per step
(prepare, gen, fuzz, selfhost, llvm, cmd, finish), so each step is its own
lease and a waiting `--ab` runs between steps instead of waiting for the
whole tier (tier 34 held the lane 1h18m; `--ab` runs waited up to 1h16m).
A Mac tier step the memory cap stops (exit 143) runs once more at `-p=2`.
Last verified: 2026-11-01

- `OFF_WAIT_MIN=N` (offload.sh, heavyd `lease -wait=N`): when no lane slot
  came in N minutes the job is not run and offload.sh exits 77. For
  coverage steps that should be skipped rather than waited for: the full
  tier's linux llvm and cmd steps use 20 and are named "SKIPPED" in the
  tier's offload line (lead, 2026-11-06 bottleneck review: tier 54's
  linux LLVM step waited 1h33m for every slot of the second host behind a
  6 hour soak and held the 0.41.0 gate). Last verified: 2026-11-06.
- Soaks and other runs of hours run outside the lane, with
  `OFF_NO_LANE=1` (offload.sh's own ticket queue, which backfills around
  an exclusive job): in a lane a plain long job holds its slot until it
  ends, and the exclusive jobs queued behind it (`--ab`, twin-tg.sh) wait
  the whole time (Xo Lang 2, 2026-11-06: a 6 hour soak in lane pass2
  held slot 2 from 13:26, and two exclusive jobs waited 48 minutes before
  the lead moved them to the first host). The same holds for any plain
  run expected past 30 minutes, such as a repro loop waiting for a rare
  hang (lead, 2026-11-06 bottleneck review: an amqp hang loop held the
  first host's lane 14:29 to 15:15 while three exclusive jobs and the
  tier's host step waited up to 1h32m behind it): send it with
  `OFF_NO_LANE=1`. Last verified: 2026-11-06.
- A plain job that carries its own inputs (no files only one host has;
  a branch goes as a bundle `--put` to both arm64 hosts, made from the
  commit both have: gotchas/offload-bundle-base.md, since a bundle inside
  the script passes Linux's 128 KB argument limit past a few commits)
  goes with `OFF_LANE='arm64|pass2'`, so heavyd starts
  it on whichever arm64 host frees first (`arm64*` alone never reaches
  the second host, whose lane `pass2` stays out of it for `--ab` runs;
  lead, 2026-11-07: Xo Lang 1's -O2 bisect waited on the first host with
  `arm64*` while the second had free slots); pin a host (`OFFLOAD_ENV=`) only when
  the job needs files already there, and do not chain plain work behind
  one's own exclusive job in a script pinned to one host (lead,
  2026-11-06 bottleneck review: pn-host.sh waited 53 minutes on the first
  host, ot1-host.sh 50 and pn-host.sh 49 on the second, each behind an
  exclusive A/B while the other host had free slots). Last verified:
  2026-11-06.
- A run with `OFF_NO_LANE=1` leaves a marker in `~/xo-lead/locks/nolane`
  (its PID, host file and name), and heavyd starts no exclusive lease on
  that host while it runs: the exclusive job shows "a run with no lane is
  on this host (NAME)", plain leases behind it go on, and a lease that
  matches another lane (`arm64*`, ab-host.sh) starts there. Send a
  timing job pinned to a host (`OFFLOAD_ENV=`) to the other host while a
  soak runs (lead, 2026-11-06: json32-ab.sh took the second host's
  exclusive lease 19:53 to 20:49 beside Xo Lang 2's 0533 soak, and the
  soak's p99 rose from 2 ms to 291 ms with its client sockets 95 to 204
  seven minutes later, so the soak did not clear 0533). Last verified:
  2026-11-06.

## The third linux/arm64 host

A container on the Mac Studio itself (owner, 2026-11-09), lane `pass3`,
settings in `~/xo-lead/offload-arm64c.env` (ssh to a local port). Docker
Desktop 4.99.0 (4.47.0 did not start on macOS 27.2) with 12 CPUs, 32 GB,
1 GB swap and a 250 GB disk; the container `xo-host3` runs
`golang:1.26.1-trixie` with `--init --restart unless-stopped --security-opt
seccomp=unconfined --cap-add=SYS_NICE --ulimit rtprio=99` and the named
volume `xo-host3-work` on /work. Same setup as the hosts above (openssh,
the user xo, LLVM 23.1.3 in /opt/llvm checked against the release's
SHA-256, clang.cfg, xo-cc, /etc/profile.d/xo.sh, tini, the 0.44.0 seed),
plus what offload.sh needs on a fresh /work: /work mode 1777 and
`offload.q`, `offload.runs` and `offload.lock*` owned by xo (without them
a job fails with "Permission denied" on /work/offload.runs). A lane for it
is one line in `heavyd-lanes.conf` (`pass3 3 2 offload-arm64c.env`); a job
asking for a lane no line names waits forever. First gate (selfbuild of
main): 35 s. Use it for pass/fail runs with `OFFLOAD_ENV=offload-arm64c.env`;
it shares the Mac's cores, so no timing runs. No WebAssembly tools yet.
Last verified: 2026-11-09.

## The Windows arm64 VM (owner, 2026-11-09)

Windows 11 Pro ARM (26100) in VMware Fusion 26H1 (free personal use) on
the Mac Studio: 8 CPUs, 16 GB, a 200 GB growable NVMe disk, NAT
networking (the Mac reaches it on Fusion's private vmnet8 address), UEFI
Secure Boot and a vTPM, so Fusion encrypts the VM's TPM files
(`vmx.encryptionType = partial`): `vmrun` needs `-vp PASSWORD`. Settings
are private: `~/xo-lead/offload-winarm.env` (the offload host, lane
`winarm`, `OFF_KIND=win`) and `~/xo-lead/winarm-vm.env` (the .vmx path and
the VM password, for `vmrun -T fusion -vp ... start VMX nogui`). The lead
logs in with the key `~/.ssh/xo_winarm` (in
`C:\ProgramData\ssh\administrators_authorized_keys`, since xo is an
administrator); offload.sh uses the password. Developer mode is on (git
symlinks without elevation). Installed: OpenSSH Server, PortableGit
2.56.0.4 arm64 in `%USERPROFILE%\tools\git`, Go 1.26.1 windows/arm64 in
`tools\go` (both checked against their published SHA-256), `xo-ci\off`.
Git Bash's runtime is x64 under emulation (`uname -m` says x86_64); Go and
the jobs' programs run natively. No xo seed for windows/arm64 yet, so it
runs programs cross-built on the Mac. Fusion's "vmware-vdiskmanager -x"
cannot grow a disk of an encrypted VM ("Failed to analyze snapshot
chain"); an empty disk was replaced with a new 200 GB one instead.
Last verified: 2026-11-09.
