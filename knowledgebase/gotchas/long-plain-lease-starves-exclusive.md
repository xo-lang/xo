# A long plain lease beside a waiting exclusive holds it for hours

Status: Observed (2026-11-18, amd64 lane)

**Symptom.** Three exclusive timing runs waited 1.5 to 2.5 h on the amd64
lane while a 540-minute websocket fuzz ran there.

**Cause.** heavyd lets plain leases start beside a waiting exclusive when
the exclusive cannot start anyway (a no-lane run ahead of it, its hold run
out, backfill, a fit before a running limit). A plain lease with a long
-max that starts then keeps the lane from the exclusive until it ends.

**Fix.** heavyd (task #339) refuses such a plain lease while an exclusive
waits on the lane: -max over 120 minutes (HEAVYD_EXCL_PLAIN_MAX_MIN), or
over 180 beside a no-lane wait (HEAVYD_EXCL_PLAIN_NOLANE_MAX_MIN, so
offload.sh's default -max=180 still keeps a host busy behind a soak); 0
turns a cap off. The refusal names the cap: split long work into -max
chunks within it (stdproven: SP_SHARD_MIN). Test: TestExclWaitRefusesLongPlain.

**The other side (observed 2026-11-19, task #409).** "Refuses" above means
holds back: the capped lease waited silently while exclusives kept
queuing. offload_test.sh, at offload.sh's default -max 180, waited 2h13m on
pass2, and mcachk.sh 26 minutes, behind stdproven exclusives. Since #409
heavyd refuses a plain lease the cap has held back on every lane it takes
for 30 minutes (HEAVYD_EXCL_PLAIN_REFUSE_MIN, 0 turns it off): the client
exits 80 (LeaseExitPlainCap) with the reason, and offload.sh does not fall
back. Set OFF_MAX_MIN to 120 or less for pass/fail runs on hosts 2 and 3.
Test: TestPlainCapRefused.
