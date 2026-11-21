# 0735: Workstream as the control plane: intents, the runner, hosts, progress, logs, outages

Status: Accepted (owner, xw decision #246, 2026-11-21; written by xw-1,
task #598). Part of
[0732](0732-one-scheduler.md), and steps 3 and 6 of its migration.
Scope: xw decisions #243 (Workstream is the control plane) and #244
(nothing in the control plane runs in Docker).

## Context

Agents and the lead drive machines by hand:

- ssh through offload.sh;
- bundles sent and `--get` copies fetched back;
- env files in ~/xo-lead that hold host addresses and passwords;
- hand-written host jobs.

Detail about a failure is lost when a host's /work is cleaned, which
tier-keep (#596) patches one step at a time. Hang detection reads log
silence (hangcheck.sh: 15 minutes for a gate, 20 for a slot), so a
quiet but healthy job raises an alert, and a busy log hides a stuck job.

## Decision

### Intents in, results out

Agents and the lead submit intents. The core turns them into jobs
(0733), runs them, and keeps the results.

- `xw run tier --rev REV`, `xw run gate WT [STEP...]`, `xw run tests
  --rev REV PKG [-run RE]`, `xw run bench --ab A B`: each returns a
  run id at once and sends an xw message when the run ends, with its
  verdict, failing lines and next action (as offload-gate's message does
  today).
- `xw result RUN [--detail] [--log STEP]` returns the verdict. With
  `--detail` it returns the failed step's detail files (kept by the
  core, below). With `--log` it returns one step's log.
- **The escape hatch.** `xw shell HOST --for 30m --why TEXT` opens a
  leased, recorded shell. The lease holds the fit a debug session needs
  (2 cores and 4 GB by default) and ends at its time. It shows in `xw
  status` and raises an alert if it is held past its time. It is the
  only way to reach a host by hand.

### The runner

`xw run-agent` is one small Xo binary on every machine, including the
Mac. It is installed and upgraded by the core.

- It talks to `xw serve` over one ssh connection that the core opens,
  using keys the core holds. Agents hold none.
- It starts each job in its own process group (cgroup on Linux), with
  the job's env and work directory.
- It sends a probe every 10 s: the group's CPU time, RSS, child process
  count and newest child, bytes written to the log, the host's load,
  memory and disk.
- It stops a job only when the core tells it to, and only that job's
  group, by the PID it started. It never signals by name or pattern, and
  never touches a process it did not start.
- When the core is unreachable, it keeps running its jobs. It buffers
  probes and results on local disk, and replays them on reconnect.

### Progress and hang detection

A job is **stalled** when, over its type's stall window (default 10
minutes, scaled by the parent's load as in 0733), all of these hold:

- the group's CPU time did not grow;
- no child process started or ended;
- the log did not grow.

A job is **slow** (one notice, not a hang) when it runs past its
predicted time with CPU still growing.

A stalled job gets one `stall` alert with the probe's last values. With
the type's `stall: stop` policy it is stopped and its detail kept. A
quiet log alone is never a hang.

### Logs, artifacts and the failed step's detail

- Each job's log streams to `~/xo-lead/runs/RUN/STEP.log` on the Mac
  and is compressed when the job ends.
- A job type names its detail globs, for example
  `/work/tierN.*.txt` or a test's `-json` output. When a step fails, the
  runner copies those files before cleanup; when it passes, it copies
  none.
- Retention is 14 days for runs that passed and 60 days for runs that
  failed, then the files are deleted. Each run's files are in `xw
  result`.

### Hosts: logins, credentials, provisioning, upgrades, health

- **Credentials.** They live only in the core's store: the macOS
  keychain on the Mac for secrets, and machine rows for addresses. They
  are never in a file an agent reads, and never printed. The env files
  in ~/xo-lead go away at migration step 6.
- **Provisioning.** The Xo plugin's recipes (today's host-setup/) run
  through `xw host provision HOST`. They are idempotent and record what
  they installed. `xw host upgrade HOST` updates the toolchain listed in
  host-toolchain.txt and rechecks parity, as offload-gate checks it
  today.
- **Health.** A machine is up while its probes arrive. After two missed
  probes it is suspect: no new jobs, and the plan is redone. After 5
  minutes it is down: its jobs are requeued and one `machine-down`
  notice is sent.
- **Docker.** Container hosts (hosts 3 and 4) are children of
  docker-vm. When Docker stops, docker-vm goes down with a single notice
  and no alert per job (#244). When Docker comes back, the core restarts
  the containers' sshd if needed (host 3's PID 1 is `sleep infinity`,
  offload.md) and brings the rows back up.
- **Cleanup.** Per host disk caps (today's offload-prune.sh and
  gocache-trim.sh rules) are machine policy rows. The runner trims
  between jobs, never under a running build. This keeps the gotcha that
  "gocache trim races builds".

### Surviving reboots and its own outages

- `xw serve` runs under launchd with KeepAlive on the Mac, as heavyd
  does (org.xolang.heavyd). heavyd's scheduling moves into `xw serve`,
  so there is one daemon.
- SQLite (0734) lives on local disk and survives a reboot. Its WAL
  recovers on open. Nothing it needs is in /tmp or Docker.
- **On restart**, `xw serve` reads the jobs table. Each running job is
  adopted, not restarted: the runner on its host reports the jobs it
  still holds. The jobs that were queued keep their place, as 0451's
  tickets do. A job whose host rebooted is requeued.
- **While `xw serve` is down:**
  - the runners keep running and buffering;
  - reads (`xw status`, `xw jobs`) answer from the file, read only,
    marked stale;
  - writes from clients fail with a clear message and are retried by
    the hooks for 2 minutes;
  - launchd restarts `xw serve` within seconds.
- With no parallel system (#245), there is no fallback to the old locks
  once a step's cutover is done. The backup and the one-command way back
  cover the cutover itself.
- **A Mac reboot.** launchd starts `xw serve`; it reconnects to every
  runner, marks the Mac's own interrupted jobs as failed with the reason
  "reboot", and replans.

## Consequences

- Agents lose direct ssh. Every host action is recorded, and a debug
  shell is a lease that someone can see.
- The failed step's detail is kept by default, so tier-keep (#596) and
  the hand-written `--get` steps go away.
- The runner is a new binary on every host. Its upgrade path (`xw host
  upgrade`) must work when the old runner is broken, so the core can
  also reinstall a runner over plain ssh.

## Alternatives

- **Keep ssh for agents and add recording.** This is cheaper, but
  credentials stay in files agents read, and the core still cannot
  adopt or replan jobs it did not start.
- **An existing agent such as Nomad's client or a CI runner.** It would
  fit the runner's role, but it brings its own server and state, and
  that duplicates the core.
