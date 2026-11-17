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
| Every Go answer in a std corpus is computed with fused multiply-add off (gotchas/go-oracle-fma-arm64.md) | scripts/stdcorpus | `scripts/stdcorpus/main.go` (noFMA: GOFLAGS to every oracle, a fused build refused) | `scripts/stdcorpus/nofma_test.go` | gotcha:go-oracle-fma-arm64 |
| A push does not lower a std module's mark unseen | xw std marks | `workstream:alerts.xo` (std_falls; prevention at push time is #140's pushq rows, pending) | `workstream:test/pg_test.sh` | std-fall |
| Claude Code agents never kill by name or pattern (pkill, killall, kill NAME, kill $(pgrep)) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkKill) | `scripts/lead/ccguard/rules_test.ts` | |
| Claude Code agents never git stash (shared across worktrees) | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkStash) | `scripts/lead/ccguard/rules_test.ts` | |
| heavy.sh is never piped or launched with &; host tools (offload.sh, offload-gate.sh, pushq.sh) never run inside it | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkHeavy) | `scripts/lead/ccguard/rules_test.ts` | |
| No git reset --hard over uncommitted tracked changes | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkResetHard) | `scripts/lead/ccguard/rules_test.ts` | |
| Commit messages carry no em-dash and no backticks in -m | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkCommitMessage) | `scripts/lead/ccguard/rules_test.ts` | |
| A generated file is changed through its generator, never by hand | ccguard hook | `scripts/lead/ccguard/rules.ts` (checkGeneratedWrite) | `scripts/lead/ccguard/rules_test.ts` | |
| A change runs the checks its kind of change needs (the path-to-check table) | pushq, gate.sh | `scripts/lead/gate-needs.txt`, `scripts/lead/gatelib.sh` (gate_table_steps) | `scripts/lead/gatesteps_test.sh` | push-refusals |
| A released CHANGELOG section is never edited | pushq | `scripts/lead/gatelib.sh` (gate_changelog_frozen) | `scripts/lead/gatesteps_test.sh` | |
| A Breaking std change has its CHANGELOG line | pushq | `scripts/lead/gatelib.sh` (gate_breaking_unlogged) | `scripts/lead/gatesteps_test.sh` | |
| Code lands after its decision is accepted | pushq | `scripts/lead/gatelib.sh` (gate_adr_proposed) | `scripts/lead/gatesteps_test.sh` | |
| A corpus changes through its generator; a corpus change or a rebase bringing one passes stdcorpus -check | pushq | `scripts/lead/gatelib.sh` (gate_corpus_mods), `scripts/lead/pushq.sh` | `scripts/lead/gatesteps_test.sh` | |
| A program started by `xo run` does not outlive it when xo is signalled (SIGHUP, SIGTERM, SIGINT sent on; SIGALRM not yet: gotchas/alarm-xo-run-orphans-program.md) | xo run | `compiler/xo/run.xo` (exec_binary) | `cmd/xo/runorphan_unix_test.go` (TestRunSignalEndsProgram), `cmd/xo/fieldreport_test.go` (TestFieldRunForwardsStop) | hang |
| Every bug fix lands with a test that fails before it | CLAUDE.md | none | none | |
| Long fuzz sweeps go sharded to a host, never the Mac gate class | CLAUDE.md | none | none | |
| Never wait idle while a gate or push runs | CLAUDE.md | none | none | idle, idle-empty, waiting |

## Schedules

| Schedule | Runs | Every (min) | Evidence |
|---|---|---|---|
| mainwatch | launchd org.xolang.mainwatch: hourly.sh, diskguard.sh | 120 | `beat:mainwatch` |
| hourly | the main check (hourly.sh, from mainwatch) | 120 | `log:notes/lead/hourly.log` |
| ctier | launchd org.xolang.ctier: the full tier on main | 120 | `beat:ctier` |
| heavyd | launchd org.xolang.heavyd (KeepAlive) | 10 | `log:heavyd.log` |
