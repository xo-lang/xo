# A bootstrap seed older than the runtime's intrinsics fails stage s1

Status: Observed (2026-10-13, decision 0212 stage 2)

Symptom: `scripts/bootstrap-xo.sh <cached xo1> DIR` failed in stage s1
at the link: `Undefined symbols: _xo_call_c2, _xo_sys_clone,
_xo_sys_words, referenced from _xo_f_main_2erun`. The same script with a
seed built from the checkout passed (s2 and s3 identical).

Cause: the seed was an xo1 from the user cache built before decision
0206 stage 2 merged. It compiles the checkout's std, whose runtime
modules now call the new intrinsics `rt.call_c2`, `rt.sys_clone`, and
`rt.sys_words`; the old seed lowers them as ordinary `rt.` calls of C
functions `xo_<name>`, which no C runtime defines (the new compilers
emit them inline).

Fix: seed a bootstrap with a compiler that knows every intrinsic of the
checkout's std: `XO_SELF_DRIVER=0 xo build --backend=arm64 -o seed
selfhost/xo` from the same checkout (or the xo1 the checkout's `xo`
builds into its cache), not an xo1 left in the cache by an older tree.
