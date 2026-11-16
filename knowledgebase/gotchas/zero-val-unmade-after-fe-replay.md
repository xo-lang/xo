# zero_val.T unmade after a front end cache replay

Status: Observed (2026-11-16; fixed by the commit that added this file)

## Symptom

With one back-end cache shared by many programs (the hourly golden run,
749 goldens at XO_GOLDEN_JOBS=14), a build failed now and then (about 1
in 1,300 on the Mac): `arm64 backend: runtime call
\`zero_val.xo.MapLogK[Str]\`` in `xo.maph.ktolog[...]`
(http_client_responses on linux/amd64), or `fault: index 5066 out of
range for List of length 5066` in ssa.raises under add_landings_with
(net_only). The same program built alone passed, and a rebuild against
the round's final cache passed: it depends on which entries were there
mid-round. XO_FE_SPEC=0 did not stop it.

## Cause

finish_fn makes zero_val.T (the shared zero a fault exit returns for a
large struct result) when a body is built. A body the front end cache
replays does not run finish_fn, so zero_val.T could still be unmade when
add_landings made the function's unwind exit:

1. the exit's finish_fn made it there, past the end of the may_fault list
   the pass had sized (the index fault);
2. the cleanup cache (decision 0382) restored the landings of another
   program where zero_val.T was there, whose exit calls it: the body names
   no zero_val.T before its landings, so the key could not tell (the
   runtime call).

## Fix

add_landings' unwind_exit runs finish_unwind, which makes no function
(fault_zero then writes the zero inline), and cln_key hashes whether each
result's zero_val.T is there. compiler/ssa zeroval_test.xo checks both.
A cache key covers every fact the cached pass decides by, including a
function's presence that the body does not name yet.
