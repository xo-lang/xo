# The 0.11.0 seed cannot build xo once Mbed TLS is gone

Status: Observed (2026-10-15, the Mbed TLS removal's seed check)

Symptom: `XO_SEED=<0.11.0 seed> go run ./scripts/xobuild` on the
removal branch: the seed's link of xo fails with undefined
`_SecTrustEvaluateWithError`, `_CFRelease` and the other Security and
CoreFoundation functions (referenced from tlscert.xo apple_trust, which
xo links through std/http); xobuild then falls back to the newest
earlier build and succeeds, which hides it (read its "$XO_SEED failed"
line).

Cause: the 0.11.0 seed links the two frameworks only together with the
Mbed TLS library, which its needs_tls turns on when the generated code
names an `xo_tls_` symbol. Since part 18 (e978a99f) the frameworks are
called from Xo and linked by framework_args, which the 0.11.0 seed does
not have; xo's own code still named an `xo_tls_` symbol until the
removal, so the seed kept linking the frameworks by accident.

Fix: build the removal with a seed that has framework_args (0.12.0 on,
or any xo built from zero-c after e978a99f): the removal lands right
after the 0.12.0 seed. Check a seed by running it directly (its
`build --backend=arm64 --std=<tree>/std -o OUT <tree>/compiler/xo`), not
through xobuild's fallbacks.
