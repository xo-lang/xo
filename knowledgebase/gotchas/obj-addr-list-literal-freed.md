# A list literal handed to C through rt.obj_addr is freed at the end of its scope

Status: Observed (2026-10-15, the zero-C audit's B22 and B23 remnants)

Symptom: Listener.addr()'s Str turned to garbage after the next spawn,
and later runs corrupted the heap in alloc during proc.spawn
(unix_kill). Adding an rc.inc on the Str instead leaked one allocation
per call (XO_LEAKCHECK "1 live", 18 in http_tls).

Cause: an Xo export returned `rt.obj_addr([s])` for C's List[Str] of
one. obj_addr does not count a reference: the literal is dropped when
the Xo function returns, so C held a freed list whose element the next
allocation reused.

Fix: hand C the element, not a container built in Xo: count the Str once
more (`rc.inc(rt.obj_addr(s))`, the testworld.hand_str pattern), return
it and store its length through an out pointer, and build the list in C
(xo_rt.c nc_list), so the list's one reference is C's. Any Xo object
handed out by obj_addr needs its own counted reference.
