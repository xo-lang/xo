# A copied baseline xo needs third_party next to its std

Status: Observed 2026-10-14 (audit item B11, measuring the C HPACK).

**Symptom.** A copy of `bin/xo` with its `std/` (the usual way to keep
the "C" side of a before and after instruction count while the worktree
changes std) builds plain programs, but a program that uses TLS (an
https client) fails:

    the TLS library sources: NotFound(path: ".../third_party/mbedtls/xo")xo: /usr/bin/cc failed

and after copying mbedtls, the same for `third_party/fiat-crypto/src`.

**Cause.** xo finds its std at `bin/../std` and the TLS and curve C
sources it compiles into https programs at `bin/../third_party/`
(mbedtls/xo and fiat-crypto/src).

**Fix.** Copy all three next to the copied binary, from the checkout of
the baseline commit (about 84 MB together):

    mkdir -p $D/bin $D/third_party
    cp bin/xo bin/xo.key $D/bin/ && cp -R std $D/std
    cp -R third_party/mbedtls third_party/fiat-crypto $D/third_party/
