# Agent rules registry

Every rule agents follow, its one home, what enforces it and the test that
proves the enforcement (task #139, owner 2026-11-16). A rule whose
enforcement is `none` lives only in prose (CLAUDE.md or a memory) and is
the first candidate for a tool check when it is broken again.

Read by machines, so keep the shape:

- `internal/repotest` `TestAgentRulesRegistry` (in `go test -short`, so the
  hourly main check runs it) fails when a listed enforcement or test path
  is missing. Paths are repo-relative, `~/...` (checked when it exists on
  this machine's lead directory), or `workstream:PATH` (the private xw
  repository, checked when `~/godev/claude-space/xolang/workstream`
  exists). A `(name)` after a path names the function or check inside it.
- xw reads the Incident kinds column: an alert kind (main failures are the
  `main-check`, `tier` and `bisect` kinds) raised 3 times in 7 days that no
  row with an enforcement lists raises a `rule-gap` alert to the lead.
- xw reads the Schedules table: a schedule whose evidence is older than
  twice its period raises `schedule-missed` to the lead.

Last verified: 2026-11-16.

## Rules

| Rule | Home | Enforced by | Test | Incident kinds |
|---|---|---|---|---|
| Never pattern-kill processes; kill only a PID the session captured | xo-guard hook | `scripts/lead/pi/rules.ts` (checkKill) | `scripts/lead/pi/rules_test.ts` | |
| Never git stash; never commit in the main checkout; worktree removal only of own worktrees | xo-guard hook | `scripts/lead/pi/rules.ts` (checkGit) | `scripts/lead/pi/rules_test.ts` | |
| Publishing, tags and pushes to the public org are the owner's | xo-guard hook | `scripts/lead/pi/rules.ts` (checkPush) | `scripts/lead/pi/rules_test.ts` | |
| CI runs and AWS writes are the owner's or the lead's | xo-guard hook | `scripts/lead/pi/rules.ts` (checkOwnerOnly) | `scripts/lead/pi/rules_test.ts` | |
| Agent configuration and lead state are the lead's to change | xo-guard hook | `scripts/lead/pi/rules.ts` (checkWritePath) | `scripts/lead/pi/rules_test.ts` | |
| Secrets are never printed or read into a session | xo-guard hook | `scripts/lead/pi/rules.ts` (checkSecretText) | `scripts/lead/pi/rules_test.ts` | |
| rm -r only of a checked, non-protected path | xo-guard hook | `scripts/lead/pi/rules.ts` (checkRm) | `scripts/lead/pi/rules_test.ts` | |
| Every build, test and repro runs through heavy.sh | xo-guard hook | `scripts/lead/pi/rules.ts` (checkHeavy), `scripts/lead/heavyd/heavy.sh` | `scripts/lead/pi/rules_test.ts`, `scripts/lead/test-heavy.sh` | |
| The Mac gate class is for runs under about 2 minutes; long runs go to a host | xw alert | `workstream:work.xo` (wrong_class), `workstream:alerts.xo` (wrong_class_quiet) | `workstream:test/work_test.sh` | wrong-class |
| A measurement's drain never holds waiting gates past 5 minutes | heavyd | `scripts/lead/heavyd/daemon.go` (phYield) | `scripts/lead/heavyd/scenario_test.go` (TestScenario31DrainYieldsToWaitingGate) | |
| Push only through pushq.sh, gated, squashed; holds respected | pushq | `scripts/lead/pushq.sh` | `scripts/lead/pushq_hold_test.sh` | push-refusals, push-refused-twice, push-held |
| A failure on main is bisected to its commit and fixed forward by its owner | hourly.sh, ctier.sh | `scripts/lead/mainbisect.sh`, `scripts/lead/mainbisect-remote.sh` | `scripts/lead/mainbisect_test.sh` | main-check, tier, bisect |
| Offload hosts keep /work free; leaked bundles are removed | offload tools | `scripts/lead/offload-prune.sh`, `scripts/lead/stdproven-offload.sh` (sp_end), `workstream:alerts.xo` (host_disk_low) | `scripts/lead/offload-prune_test.sh`, `scripts/lead/stdproven_offload_test.sh` | host-disk-low, disk-low |
| A lease name is not resubmitted in a loop | xw alert | `workstream:alerts.xo` (retry_storm) | `workstream:test/pg_test.sh` | retry-storm |
| Long waits and blocked leases go to the digest unless they grow | xw alert | `workstream:alerts.xo` (wait_rising) | `workstream:test/pg_test.sh` | long-wait, lease-blocked, wait-rising |
| Assigned tasks and decisions are acked in the next tool round | xw inbox | `scripts/lead/xw-cc-hook.sh` | `scripts/lead/xw-cc-hook_test.sh` | unacked, unacked-task, unacked-idle |
| Questions for the owner or the lead go through xw ask | xw | `workstream:asks.xo` | `workstream:test/work_test.sh` | |
| A pushed task is marked done, or pushed as a part, not left pushed | pushq, xw | `scripts/lead/xwhook.sh` (xw_task_push: PUSHQ_TASK, PUSHQ_PARTIAL), `workstream:std.xo` (push_partial: auto-partial for a multi-module task) | `scripts/lead/xwhook_test.sh`, `workstream:xw_test.xo` | task-pushed |
| An agent at its prompt while its run goes starts its next queued task | xw alert | `workstream:alerts.xo` (agent_first_kind: the agent's inbox first, the lead on a repeat), `workstream:inbox.xo` (AGENT_ALERTS) | `workstream:test/work_test.sh` | waiting |
| Work outside an agent's charter needs the lead's ok; a bug fix does not | xw check | `workstream:work.xo` (off_charter) | `workstream:test/work_test.sh` | off-charter |
| A stuck gate, spinning process or dead Docker is found and stopped | heavyd, hangcheck | `scripts/lead/hangcheck.sh`, `scripts/lead/heavyd/daemon.go` (gate and slot timeouts, stall stop) | `scripts/lead/heavyd/scenario_test.go` (TestScenario17Stall) | hang |
| A job quiet by design prints a heartbeat, so the hang check reads it as alive (the corpus check) | gatelib.sh | `scripts/lead/beatrun.sh`, `scripts/lead/gatelib.sh` (gate_corpus_run), `scripts/stdcorpus/main.go` (beat) | `scripts/lead/gatesteps_test.sh` (corpus run: heartbeats), `scripts/stdcorpus/beat_test.go` | hang |
| pushq's corpus and stdcov checks refuse only what the push is to blame for; stale corpora or uncovered entries already on main are a note (gotchas/pushq-corpus-check-blocked-on-main-stale.md) | pushq | `scripts/lead/gatelib.sh` (gate_corpus_blame, gate_stdcov_blame) | `scripts/lead/gatesteps_test.sh` | gotcha:pushq-corpus-check-blocked-on-main-stale |
| Every Go answer in a std corpus is computed with fused multiply-add off (gotchas/go-oracle-fma-arm64.md) | scripts/stdcorpus | `scripts/stdcorpus/main.go` (noFMA: GOFLAGS to every oracle, a fused build refused) | `scripts/stdcorpus/nofma_test.go` | gotcha:go-oracle-fma-arm64 |
| A push does not lower a std module's mark unseen | xw std marks | `workstream:alerts.xo` (std_falls; prevention at push time is #140's pushq rows, pending) | `workstream:test/pg_test.sh` | std-fall |
| Claude Code agents never kill by name or pattern (pkill, killall, kill NAME, kill $(pgrep)) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkKill) | `scripts/lead/ccguard/rules_test.ts` | |
| Claude Code agents never git stash (shared across worktrees) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkStash) | `scripts/lead/ccguard/rules_test.ts` | |
| heavy.sh is never piped or launched with &; host tools (offload.sh, offload-gate.sh, pushq.sh) never run inside it | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkHeavy) | `scripts/lead/ccguard/rules_test.ts` | |
| No git reset --hard over uncommitted tracked changes | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkResetHard) | `scripts/lead/ccguard/rules_test.ts` | |
| Commit messages carry no em-dash and no backticks in -m | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkCommitMessage) | `scripts/lead/ccguard/rules_test.ts` | |
| On darwin no alarm or timeout wrapper (perl -e 'alarm N; exec @ARGV', timeout, gtimeout) around `xo run`: build, then time BIN (gotchas/alarm-xo-run-orphans-program.md) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkAlarmRun) | `scripts/lead/ccguard/rules_test.ts` | |
| A generated file is changed through its generator, never by hand | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkGeneratedWrite) | `scripts/lead/ccguard/rules_test.ts` | |
| No commit, amend, rebase or checkout in a worktree with a running gate or queued push (memories No rebase under a gate, No commit under a gate, One worktree per queued push) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkBusyTree, busyIn), `scripts/lead/ccguard/hook.ts` (realBusy) | `scripts/lead/ccguard/rules_test.ts` | |
| A lease whose starting chain is stopped frees its slots (memory Chain kill orphans lease): the lease client watches its parent chain and stops its job, exit 129, logged ORPHAN; nohup or HEAVYD_DETACHED=1 opts out | heavyd lease client | `scripts/lead/heavyd/lanes.go` (chainOf, chainBroken) | `scripts/lead/heavyd/lanes_test.go` (TestLeaseOrphanedChain) | |
| No delete list built from ls (aliased -F adds / and *; memory ls-classify): find DIR -maxdepth 1 -name PATTERN instead | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkLsDelete) | `scripts/lead/ccguard/rules_test.ts` | |
| A change runs the checks its kind of change needs (the path-to-check table) | pushq, gate.sh | `scripts/lead/gate-needs.txt`, `scripts/lead/gatelib.sh` (gate_table_steps) | `scripts/lead/gatesteps_test.sh` | push-refusals |
| One --ab per agent at a time; --ab runs 3 rounds by default (memories One --ab at a time, 3 round --ab) | ab-host.sh (exit 2); xo bench default | `scripts/lead/ab-host.sh` (one per agent); `compiler/cli/bench` (rounds 3) | `scripts/lead/offload_gate_test.sh`; `compiler/cli/bench/bench_test.xo` | |
| An exclusive host 1 job is at most 30 minutes; split longer ones (memory Host 1 chunks); no exclusive lease beside a soak (memory Soaks before exclusive) | offload.sh (exit 2); heavyd lease waits | `scripts/lead/offload.sh` (host 1 exclusive check); `scripts/lead/heavyd/lanes.go` ("a run with no lane is on this host") | `scripts/lead/offload_gate_test.sh`; `scripts/lead/heavyd/lanes_test.go` | |
| An offload gate's OFF_MAX_MIN is never below its steps' budget (memory Offgate budget) | offload-gate.sh (exit 2) | `scripts/lead/offload-gate.sh` (OFF_MAX_MIN floor) | `scripts/lead/offload_gate_test.sh` | |
| A released CHANGELOG section is never edited | pushq | `scripts/lead/gatelib.sh` (gate_changelog_frozen) | `scripts/lead/gatesteps_test.sh` | |
| A Breaking std change has its CHANGELOG line | pushq | `scripts/lead/gatelib.sh` (gate_breaking_unlogged) | `scripts/lead/gatesteps_test.sh` | |
| Code lands after its decision is accepted | pushq | `scripts/lead/gatelib.sh` (gate_adr_proposed) | `scripts/lead/gatesteps_test.sh` | |
| A corpus changes through its generator; a corpus change or a rebase bringing one passes stdcorpus -check | pushq | `scripts/lead/gatelib.sh` (gate_corpus_mods), `scripts/lead/pushq.sh` | `scripts/lead/gatesteps_test.sh` | |
| A program started by `xo run` does not outlive it when xo is signalled (SIGHUP, SIGTERM, SIGINT sent on; SIGALRM and SIGKILL not, so the ccguard row above blocks alarm wrappers: gotchas/alarm-xo-run-orphans-program.md) | xo run | `compiler/xo/run.xo` (exec_binary) | `cmd/xo/runorphan_unix_test.go` (TestRunSignalEndsProgram), `cmd/xo/fieldreport_test.go` (TestFieldRunForwardsStop) | hang |
| An offload run starts on a seed at most 100 commits behind and 180 s of xo build a job (a batch is refused past 400 commits or 600 s); an older seed asks the owner once per version | xw preflight (#157) | `workstream:preflight.xo` (check_seed, seed_standing), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| An offload run starts while Docker answers (no open DOCKER-DEAD) | xw preflight (#157) | `workstream:preflight.xo` (check_docker), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| An offload run starts on a host with at least 30 GB free (a batch is refused under 15 GB) | xw preflight (#157) | `workstream:preflight.xo` (check_disk), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| A run on main's code knows main's open failures | xw preflight (#157) | `workstream:preflight.xo` (check_main), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| A batch knows which tier leases hold the evidence lanes | xw preflight (#157) | `workstream:preflight.xo` (check_tier), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| An agent's offload runs stay within OFF_MAX_QUEUED (a batch past it is refused) | xw preflight (#157) | `workstream:preflight.xo` (check_caps), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| A run on hosts 3 or 4 or the Mac keeps the Mac under the gate threshold (load 48, 55% free; hosts 3 and 4 are containers on it) | xw preflight (#157) | `workstream:preflight.xo` (check_mac), `scripts/lead/xwhook.sh` (xw_preflight) from offload.sh, offload-gate.sh, stdproven-offload.sh | `workstream:xw_test.xo`, `workstream:test/pg_test.sh`, `scripts/lead/xwhook_test.sh`, `scripts/lead/stdproven_offload_test.sh` | |
| The full tier runs the LLVM dev goldens (TestDevGoldensLLVM, every golden program built with the LLVM backend) on a whole host | ctier | `scripts/lead/ctier.sh` (CTIER_STEPS default: devllvm), `scripts/gate.sh` (devllvm), `scripts/lead/offload-gate.sh` (devllvm takes the host) | `scripts/lead/ctier_split_test.sh` | tier |
| Every bug fix lands with a test that fails before it | CLAUDE.md | `workstream:work.xo` (checks: bug-no-test), `workstream:ingest.xo` (has_test_file) | `workstream:test/work_test.sh` | bug-no-test |
| Long fuzz sweeps go sharded to a host, never the Mac gate class | CLAUDE.md | `scripts/lead/heavyd/heavy.sh` (exit 67) | `scripts/lead/heavy_fuzz_test.sh` | |
| Never wait idle while a gate or push runs | CLAUDE.md | `workstream:work.xo` (checks: idle, idle-empty, waiting) | `workstream:test/work_test.sh` | idle, idle-empty, waiting |

## Schedules

| Schedule | Runs | Every (min) | Evidence |
|---|---|---|---|
| mainwatch | launchd org.xolang.mainwatch: hourly.sh, diskguard.sh | 120 | `beat:mainwatch` |
| hourly | the main check (hourly.sh, from mainwatch) | 120 | `log:notes/lead/hourly.log` |
| ctier | launchd org.xolang.ctier: the full tier on main | 120 | `beat:ctier` |
| heavyd | launchd org.xolang.heavyd (KeepAlive) | 10 | `log:heavyd.log` |
