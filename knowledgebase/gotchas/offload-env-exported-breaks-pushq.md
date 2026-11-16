# An exported OFFLOAD_ENV makes pushq's repo checks fail after a passing gate

Observed 2026-11-16 (azure-1). Status: Observed.

**Symptom.** A chain of `export OFFLOAD_ENV=offload-arm64c.env`, then
`offload-gate.sh`, then `pushq.sh --gated`, passed the gate but then stopped
at pushq with "fast repo checks failed". internal/repotest
TestOffloadGateLease failed with "FAIL a memory gate goes to host 4 first:
got [offload-gate: no .../lead/offload-arm64c.env]". It happened twice, on
host 3 and host 4.

**Cause.** pushq's fast checks run internal/repotest, and
TestOffloadGateLease runs offload_gate_test.sh against a temporary lead
directory. The exported variable passes through to that test, so the gate
under test looks for the named env file in the temporary directory instead
of picking a host itself.

**Fix.** Set OFFLOAD_ENV, and OFFGATE_AGENT too, as a prefix on each
offload command (`OFFLOAD_ENV=offload-arm64c.env offload-gate.sh WT`,
`OFFLOAD_ENV=... stdproven-offload.sh ...`), and never export it in a shell
that later runs pushq. After the failure, rerun `pushq.sh WT --gated`
without the variable. If the gate record is still current it pushes;
otherwise run the gate again.
