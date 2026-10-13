# Every socket waiter must arm the AFD poll on Windows

Status: Observed (2026-10-12), fixed in std/llrt/xo_h2.c (h2_wait).

Symptom: with the AFD poller (decision 0197 stage 5) the http_tls golden
timed out on its second and fourth HTTPS request (`echo: Timeout`,
`moved: Timeout`); every other golden passed, and so did http_tls with
`XO_WINPOLL=wsapoll`.

Cause: the poller learns that someone waits from the waiter: pd_arm
starts a poll of the socket for the directions waited in. The HTTP/2
connection loop (h2_wait) sets `pd->rwait` itself instead of calling
pd_wait_until, so no poll was armed and its readiness never came.
WSAPoll scanned every registered descriptor's waiters on each loop, so
it found them anyway.

Fix: code that sets `pd->rwait` or `pd->wwait` itself calls `pd_arm(pd)`
with `pd->mu` held when `ev.afd` is set, and pokes the poller otherwise,
as pd_wait_until does.
