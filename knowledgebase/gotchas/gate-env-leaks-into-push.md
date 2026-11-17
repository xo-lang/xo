# A gate's settings leak into the push that follows it

Status: Observed (2026-11-16, twice; fixed by #169).

Symptom: a push run in the same shell as an offload gate fails its fast
checks in TestOffloadGateLease with a planned "lease 40" where the test
expects the default budget.

Cause: `OFF_MAX_MIN=40 offload-gate.sh ...` exported in a chain or script
stays in the environment for the `pushq.sh` that follows, and the fast
checks run the lead tool tests, which read OFF_MAX_MIN.

Fix: pushq.sh now unsets every `OFF_*`, `OFFGATE_*`, `OFFLOAD_*` and
`GATE_*` variable before it runs anything and logs the names in
pushq.log ("cleared gate-only settings: ..."). Test:
scripts/lead/pushq_env_test.sh. Still set gate settings on the gate
command only (`VAR=x offload-gate.sh`), not with `export`.
