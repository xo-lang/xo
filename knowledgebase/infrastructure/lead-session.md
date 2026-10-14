# Lead session: resume point

Last verified: 2026-10-14

How the lead session (merging agents' work, the EC2 queue, seeds) stood
on 2026-10-14 at 10:45, so a new session can resume (owner,
2026-10-14). The tools are in scripts/lead (its README says how to run
them).

The main checkout is ~/godev/claude-space/xolang/xo
(moved from claude-space/codelang on 2026-10-14; the lead scripts use the
new path).

## Main

- origin/main 310d3850, version 0.10.0. Seed 0.10.0-23ce8ce2 is in the
  seed bucket for all five platforms and seeds/latest.txt names it
  (decision 0216).
- The fixed point on 37bcf829 holds (s2 = s3, 52,710,416 byte binaries,
  checked 2026-10-14 09:12).
- Merged on 2026-10-14: the Windows Proc.run fix (Windows goldens 154 of
  154); the amd64 wasm regexp_table trap parked as a known issue
  (gotchas/wasm-regexp-table-intermittent-trap.md); W1 (the scheduler's
  C path deleted); fast builds rounds 2 and 3 and stage 2 A and B
  (decision 0220: clang -O0 development builds, joined releases, per
  part objects and the per function back-end cache, XO_BACKEND_CACHE);
  seed 0.10.0 and the C Rand deleted; README Install and seed chain hops
  5e21dc31 and 23ce8ce2 (the hops are not yet run end to end); zero-C
  audit items B10, B12, B11 (HTTP client parser, response writer, client
  HPACK), B05 stage 1 (decision 0221), B02 part 1 (rt.static_word), B09
  (decision 0222), B03 (stdio, exit, env, args), B16 part 1 (POSIX Fs),
  B13 stage 1; cache pruning (decision 0223); faster -short (repo 35.0 s
  to 15.7 s) and the xo-build temp directory leak fixed.

## In flight

No agents are running (owner, 2026-10-14 10:00: start no more; all
finished work is merged). Since 09:15, fast builds stage C (67fe56a2:
one-module edit rebuild of xo 30.60 s to about 11 to 12 s on the Mac)
and shrink wrapping with tail calls (310d3850, decision 0224: self
build 0.985 of the instructions) landed.

Held on branches, not merged (both correct, both miss decision 0180's
bar on arm64 only):

- B14, the test world part 1 in Xo (916 C lines deleted):
  worktree-agent-adf33485f07a2fcac at 0726c7ba. rand 1.100 and gen
  1.094 of the C after shrink wrapping
  (bench/results/shrink-wrapping-2026-10-14.md:89-91). Next: inlining
  in rand.next_int.
- B13 stage 2, the list objects in Xo: re-landed on branch b13-stage2
  (decision 0226, arm64 generated code calls obj.xo with no C level);
  meets the bar on the Mac, 0.883 to 1.021 of the C on arm64 and LLVM,
  self build 1.000 (bench/results/string-list-objects-2026-10-14.md,
  "Stage 2 re-landed"); waiting for CI auto and the lead's merge. The
  string objects stay C.

## Next, in order

1. B14 (inlining in rand.next_int) and B13 stage 2 (the allocation
   path's C entry level), then recheck both against the bar.
2. Cut seed 0.11.0 (scripts/lead/seeds), then delete the C the 0.10.0
   seed still calls: xo_static_word, xo_ct_barrier, B03's xo_print
   forwards, xo_exit, xo_env_*, xo_str_rune_count
   (gotchas/runtime-c-the-seed-still-calls.md).
3. B02 part 2 (needs 0.11.0), B16 part 2 (Windows), B05 stage 3 (glibc
   and macOS arm64), B06, B04, then the audit's order
   (bench/results/zero-c-audit-refresh-2026-10-13.md).
4. Open with the owner: a committed check pipeline (named check
   profiles that agents request and the merge gate reuses).

## Working rules in force

- Test on arm64 only (CLAUDE.md); a fix for an x86 or Windows only bug
  gets one targeted job on a warm machine of that platform.
- Agents' pass/fail runs go to on demand CI (`gh workflow run ci.yml
  -f ref=BRANCH -f suite=auto`, decision 0225, ci.md); the EC2 queue
  scripts are retired (scripts/lead/README.md). The Mac's two heavy
  slots are for measurements.
- Merge reports in batches with one gate; never run two agents on the
  same C file at once.
