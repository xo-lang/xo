# 0451: heavyd lanes: the offload hosts, the merge queue and the Mac in one scheduler

Status: Accepted (owner, 2026-10-31: "Can we run all the tests through
heavy? Then have multiple lanes? And can the merge queue and other
features go in there too?", then "Yes with the heavy stages" to the
staged plan below). Stages 1 to 5a landed 2026-10-31: lanes in heavyd
(041e4f0a9), offload.sh in the host lanes (e07848322), pushq in the merge
lane (8e7a90ea6), routing (9de7d3625), the win and docker lanes. Stage 5b
(deleting the hosts' own queues and pushq's lock) waits until the lanes
have run a day with no fallback in heavyd.log or the scripts' output.

## Context

heavyd (decision 0442) schedules the Mac's heavy slots, gates and
measurements. Four other queues schedule the rest, each with its own
rules, status view and bugs:

- offload.sh: the arm64 host's ticket queue (on the host, under flock),
  exclusive runs for `--ab`, backfill beside a long run, the per-agent cap
  (counted by walking `ps` for pid files), the ssh drop watchdog;
- amd64-run.sh and offload.sh's wsl path: a flock per slot on the amd64
  host (an exclusive run took only one slot until 5910733a5);
- pushq.sh: its own push lock directory with stale lock handling;
- Docker and Windows runs: no queue at all.

Each has had a hang or fairness bug in the last week (the cap that
counted both hosts, the amd64 exclusive slot, the 3-hour job that held a
slot while 15 waited), and there is no single view of who waits for what.

## Decision

heavyd owns every queue as lanes. A lane is a named pool of slots that
heavyd leases in arrival order; the holder runs its own command (the
daemon only says when), as heavy.sh's client already does.

- `$LEAD/heavyd-lanes.conf` lists the lanes: `NAME SLOTS [CAP]`, read on
  every pass, so an edit needs no restart. With no file: `arm64 2 2`,
  `amd64 2 0`, `merge 1 0`. A second arm64 host is one more line (and its
  env file for the ssh side).
- `heavyd lease [-x] [-long] [-name=N] LANE -- CMD...` waits for a slot,
  runs CMD with `HEAVY_LANE` and `HEAVY_LANE_SLOT` (the slot numbers) set
  and the caller's stdin and output, and exits with CMD's status. It
  exits 75 when no daemon answers, so the caller falls back to its old
  lock; 76 when no lane matches (the caller falls back the same way); 5
  when the cap refuses.
- The rules, ported from offload.sh with their reasons: arrival order;
  `-x` (exclusive) takes every slot and blocks the leases behind it;
  backfill: while an exclusive lease waits, a plain lease may take a free
  slot once a `-long` lease (its own limit 180 minutes or more) has held
  its slot 20 minutes; CAP is per agent (the Claude session above the
  client), counting waiting and held leases.
- `-p` (OFF_PRIORITY=1 for offload.sh) waits ahead of the leases without
  it (owner, 2026-10-31: a fix's linux confirmation waited an hour behind
  two 10-round --ab runs); it never stops a running lease.
- A daemon restart keeps places: a waiting client queues again with its
  ticket (its arrival time), a running one is adopted with its slots.
- `heavy.sh --status` lists every lane, its holders and its waiters with
  the reason each waits.

Stages, each pushed and running live before the next:

1. Lanes in heavyd (this push): the lease protocol, the config, the
   rules, the status view, tests. No caller uses them yet.
2. Host lanes: offload.sh and amd64-run.sh take a lease (`arm64`,
   `amd64`, `-x` for OFF_EXCLUSIVE, `-long` for OFF_MAX_MIN >= 180)
   around their ssh step; the host side keeps a plain flock per slot as a
   safety net, and the host ticket queue, backfill and pid file cap go.
3. Merge lane: pushq.sh takes a `merge` lease instead of its lock
   directory; its rebase, checks and push stay in the script.
4. Routing: a run says what it needs (darwin, linux/arm64, amd64,
   exclusive) and heavyd picks the first free lane that has it, so a
   bulk run that does not need darwin goes to whichever Linux host frees
   first.
   As built: a lease asks for a spec, `NAME` (that lane), `NAME*` (every
   lane whose name starts with NAME, the first to free a slot takes it)
   or `env:FILE` (the lane of that host file); heavyd-lanes.conf's fourth
   column names each host lane's file, which the run gets as
   HEAVY_LANE_ENV and offload.sh uses. offload.sh asks for `arm64*` or
   `amd64*` unless the caller names a host file (OFFLOAD_ENV, then
   `env:FILE`): callers that `--put` files first (ab-host.sh, tier.sh,
   amd64-run.sh) name it, so the put and the run reach the same host. A
   second arm64 host: its env file in ~/xo-lead and one line,
   `arm64-2 2 2 offload-2.env`.
5. Docker and Windows lanes (5a: `win 2 0 offload-win.env` for
   OFF_KIND=win jobs, `docker 1` around scripts/docker-linux.sh's
   container run), then the per-script queues are deleted (5b).

## Consequences

- One status view and one cap across hosts; the queue rules live in Go
  with table tests (TestLaneDecide) instead of shell on two machines.
- The daemon becomes a dependency of the offload and push paths; each
  caller keeps its old lock as the fallback for exit 75, as heavy.sh
  keeps legacy-heavy.sh.
- Lease runs have no stall watch on the Mac (their CPU is on a host): the
  host's own idle watchdog (OFF_IDLE_MIN, 0442's addendum) covers them.

## Alternatives

- Keep the queues separate and fix each bug where it is: the status quo;
  four rule sets keep drifting apart.
- Move the queues to the hosts (a scheduler per host): no global cap, no
  routing between hosts, and the merge queue has no host.
- One generic job runner that also does the ssh: heavyd would grow the
  transport, the job files and the host watchdog; leases keep those in
  the scripts that already have them working.
