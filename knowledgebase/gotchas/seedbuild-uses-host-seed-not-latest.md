# The gate's seedbuild uses the host's XO_SEED, not the newest seed

Status: Observed 2026-11-15.

## Symptom

After seed 0.52.0 was uploaded (latest.txt updated, version commit on
main), a push that needs the new seed (decision 0662 part 2: `uses sync`
in std types in xo's closure) passed every gate step but seedbuild:
`std/http.xo:549:80: error XO0702: unknown effect sync` with xo 0.51.1
linux/arm64.

## Cause

scripts/gate.sh's seedbuild step builds compiler/xo with `$XO_SEED`, the
offload host's seed: the newest seed under the host's /work/seed that is
an ancestor of the commit. Uploading a seed to the bucket does not put it
there, so the host kept using 0.51.1. Then, with 0.52.0 installed, it
still picked 0.51.1: the seed's commit (381640474) was not an ancestor
of main, because pushq had rebased the version commit to 26d1e5bf8, and
the ancestor rule skips such a seed. The lead pointed the hosts' gate
seed at 0.52.0 directly.

## Fix

After a seed cut, the seed is installed under each offload host's
/work/seed (Lang 2, through the lead) before the first push that needs
it. Until then, check such a push with the new seed on the Mac
(`XO_STD=WT/std SEED/xo check ./compiler/xo`, 0 errors) and gate once the
host has it.
