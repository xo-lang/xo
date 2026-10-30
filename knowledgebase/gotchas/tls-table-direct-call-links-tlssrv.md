# A direct tlssrv call on the plain HTTP path links the TLS server

Status: Observed (2026-10-30)

## Symptom

internal/llvm TestTLSLinkedOnlyWithTLS/plain fails with "symbols of
tlssrv: true, want false" after a change to std/native/http.xo,
llrt/hconn.xo or llrt/hserve.xo, though tls13srv, tls12srv and h2srv
stay absent. Tier 28 (e40567e45) failed it on the Mac and on Linux;
tier 27 (c2bb981f5) passed.

## Cause

Decision 0244 keeps the TLS server out of a plain server by reachability:
hconn.xo and hserve.xo call TLS only through the table
http.Server.with_tls installs (srvtls.xo, cabi_tls.xo exports, hconn.xo
TO_*). A call of a tlssrv function by name from code every server
reaches links tlssrv whether or not the branch runs: 0f11729da called
tlssrv.set_deadline and timed_out in hconn.xo's read path and
hserve.xo's hc_h2 (under `if tls != 0`, which the backends do not use to
drop the callee), and edd6fd640 called tlssrv.negotiated from
http.xo's head_request (every HTTP/1.1 request). A type named from
tlssrv (tlssrv.Cert in the Server struct) links nothing; a function
call does.

## Fix

Route the call through the table: a TO_* index in hconn.xo, an export
in cabi_tls.xo (Int arguments and result, N counted up), its address in
srvtls.xo install, and rt.call_c2 or call_c4 at the site
(TO_SET_DEADLINE, TO_TIMED_OUT and TO_NEG, 2026-10-30). A value that is
not an Int travels as the record of an Any (tlssrv.neg_any), and the type
it holds lives in a leaf module (llrt/tlsneg.xo) so the reader names no
tlssrv. Run TestTLSLinkedOnlyWithTLS (about a minute: two builds) in the
gate of any change to the three files that mentions tlssrv.
