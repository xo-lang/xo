# Offload host

A Linux arm64 machine on the owner's network that runs part of the lead's
full tier, so the Mac's heavy slots serve gates and measurements (owner,
2026-10-29: the offload container instead of routine CI). The repository
names no address or password (decision 0249): they live in
`~/xo-lead/offload.env` (mode 600) as `OFF_HOST`, `OFF_PORT`, `OFF_USER`,
`OFF_PW` and `OFF_SEED`.

Last verified: 2026-10-29

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
- `scripts/lead/offload-tier.sh SHA N SEED [BUNDLE]` (run through
  offload.sh): fetches the bundle into `/work/priv`, checks SHA out into
  `/work/tierN`, builds xo (`go run ./scripts/xobuild` with `XO_SEED`, or
  the host's cached xo of an ancestor commit; never the seed chain), then
  the steps above; logs in `/work/tierN.*.txt`.

## The host's setup

A plain `golang:1.26.1-trixie` container (Go 1.26.1, Debian trixie, 12
CPUs, 39 GB) with ssh on a mapped port and password login, set up by hand
on 2026-10-29:

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
