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

`scripts/lead/tier.sh` sends the tier's commit as a git bundle (the
commits since `refs/offload/sent`, all of them the first time; the host
has no access to the private repository, and the public one's hashes
differ, since publish.sh rewrites its history) and runs
`scripts/lead/offload-tier.sh` there in the background, beside the Mac's
steps, logging to `~/xo-lead/agents/lead-tierN-off-cmd.log`:

- moved off the Mac (they fail the tier): `gen` (the Go backend), `fuzz`,
  internal/selfhost's merge tier (the wasm goldens);
- Linux coverage the Mac cannot give (reported as `linux NAME: ...`, not
  failing the tier until their known failures are fixed): `llvm`, `cmd`.

When the host does not answer, tier.sh runs every step on the Mac as
before. The Mac keeps `short`, `arm64`, `llvm`, `cmd` and `corpora-all`
(darwin coverage), the newest Go and sepcomp Docker steps.

## Tools

- `scripts/lead/offload.sh SCRIPT|- [ARG...]` runs a bash script on the
  host as user `xo` under `flock /work/offload.lock` (one job at a time)
  and exits with its status; `--put LOCAL REMOTE` copies a file. It needs
  no heavy.sh slot (only ssh runs on the Mac). Exit 3: no offload.env;
  exit 4: no answer.
- `scripts/lead/ab-host.sh BASE NEW NAME [--rounds=N] [--warmup=N] --
  self|edit PROG [FLAG...]|PROG [ARG...]` (lead, 2026-10-30: a wall `--ab` whose claim is not
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
  with `ARG...`; 10 rounds and 2 warmups by default. The table
  streams back; logs stay in the host's `abNAME.out`. No heavy.sh slot.
- `scripts/lead/offload-tier.sh SHA N SEED [BUNDLE]` (run through
  offload.sh): fetches the bundle into `/work/priv`, checks SHA out into
  `/work/tierN`, builds xo (`go run ./scripts/xobuild` with `XO_SEED`, or
  the host's cached xo of an ancestor commit; never the seed chain), then
  the steps above; logs in `/work/tierN.*.txt`.

## The host's setup

A plain `golang:1.26.1-trixie` container (Go 1.26.1, Debian trixie, 12
CPUs, 39 GB) with ssh on a mapped port and password login, set up by hand
on 2026-10-29. Recreated by the owner on 2026-10-30 after its ext4 /work
went emergency read only (at 60% of 453 GB, a filesystem error, not a
full disk): now with `--init` (docker-init as PID 1 reaps orphans; the old
`sleep infinity` PID 1 had piled up 42,652 zombies) and an 89 GB disk, so
clean up /work after each job. The lead reinstalled the list below
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
- the seed: the public 0.34.0 linux-arm64 release in `/work/seed`; after a
  minor version bump that seed cannot build, put the new release there
  and change `OFF_SEED`.

## Findings (2026-10-29)

- internal/llvm takes 30 minutes there against 46 to 56 on the loaded Mac.
- clang's generic armv8 target rejects `eor3` (SHA-3) and, before
  1b0a7dcb9, `crc32x`: `-mcpu=native` in clang.cfg.
- Ten internal/llvm tests fail only on Linux arm64 (the list in the lead's
  notes, `offload/linux-arm64-llvm-fails-2026-10-29.md`), assigned to
  Xo Lang 3.

## The linux/amd64 host

The owner's second machine (2026-10-29): a 16-core x86-64 box running
Windows with OpenSSH (password login, user `xo`, no administrator) and a
WSL2 Ubuntu 24.04 distribution for that user (28 CPUs, 62 GB from its
`.wslconfig`). Its values live in `~/xo-lead/offload-amd64.env` (mode
600) with `OFF_KIND=wsl` and `OFF_SEED` the WSL path of the 0.34.0
linux-amd64 seed (`~/xo-ci/seed/xo/bin/xo`); WSL has clang 18, lld,
llvm, build-essential, expect and Go 1.26.0 in `~/go-sdk` (on PATH from
`~/.profile`). Windows' own shell is cmd.exe, so offload.sh sends a job
there as a file: scp (`-O`, the legacy protocol, which both hosts take)
to `%USERPROFILE%\xo-ci\off`, then `wsl -e bash -l` on it under
`flock ~/xo-ci/offload.lock`.

- `scripts/lead/amd64-run.sh REV NAME [GOLDEN|all|sample ...]` sends REV
  as a bundle (the commits since `refs/offload-amd64/sent`) and runs
  `scripts/lead/offload-amd64.sh` there: xo built natively at REV with the
  seed, then `scripts/linux-goldens.sh` with `--backend=amd64` and no C
  toolchain (as scripts/docker-goldens.sh). The default `sample` is
  scripts/docker-amd64-sample.txt. Logs stay in `~/xo-ci/runNAME.out` in
  WSL. Unlike the Docker run on the Mac, nothing is emulated (no Rosetta)
  and xo is the native backend's, not a Go backend cross build.
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
