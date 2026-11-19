# Front end cache: a flag a declare sets was not the body's

Status: Observed (2026-11-18), fixed.

## Symptom

Building http_client_proxy, then http_duplex_h2tls, with one cache and `XO_BACKEND_CACHE=verify` failed on the
second build:

    XO_BACKEND_CACHE=verify: 1 function bodies' SSA differs from their front end cache entries, the first http.stream_request

The two entries differed only in their flag bits (fe_flagbits, the entry's meta after the module list): need_conc
was set in one and not the other. Found by task #353's read-set work (class guards proposal 4); on main since
the flag bits were added (decision 0271 step 1a).

## Cause

`Builder.set_flag` added a one-way flag (need_conc, uses_net) to the recording body's bits only while
`b.m.fe_log` was on, and `declare` turns the log off while it makes a callee's signature. A signature that names
a concurrency type sets need_conc in `typ`, so a body that declared that callee itself (http_duplex_h2tls) lost
the flag, and the same body recorded after another program had made the callee did not: two entries for one body.
A replay of the first in a program where nothing else sets need_conc would build without the flag.

## Fix

`set_flag` adds the bit while a body records (`b.fe_rec`) even with the log off. Test: internal/arm64
TestFrontCacheVerifyAcrossPrograms/http_duplex_h2tls (fails before: the base xo built the pair with rc 1).

## How to avoid

What a body's recording notes must not depend on `fe_log` alone: work done inside a declare is still the body's
when the body caused it. Check new recorded state with both orders of two programs that share the body.
