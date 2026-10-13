# A C ABI export the C reaches without the feature must be built in every program

Status: Observed (2026-10-13, decision 0206 stage 3)

Symptom: with the socket system calls in std/native/llrt/cabi_net.xo
(built only for a program that dials or listens, or uses std/http), the
XO_NOLIBC runtime object listed `xo_x_net_close` and `xo_x_net_nonblock`
among its undefined symbols from functions every program with timers or
processes keeps after --gc-sections: the poller's pd_end and pd_close
close descriptors (pipes too), and ev_init makes its self pipe
nonblocking. A static timer-only program would have failed to link.

Cause: a feature's marker module (cabi_<feature>.xo) is built only when
the feature's code is (importer_built), but C functions shared with
other features call its exports. In a dynamic build the C declares the
network's exports weak and falls back to the C library, which hides
this; a program linked with no C library has strong references and no
fallback.

Fix: what the poller reaches (epoll, close, nonblocking, shutdown) is in
std/native/llrt/cabi_poll.xo, which the SSA builders build in every
native program (importer_built returns true for it, in both builders);
only the sockets themselves and the resolver stay in cabi_net.xo.
Check with `nm -u` on an XO_NOLIBC object of xo_rt.c (cross compiled
with the linux/arm64 headers) which exports a change makes the C need,
and from which C functions.
