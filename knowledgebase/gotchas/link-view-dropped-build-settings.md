# Checker.link_view dropped the build's settings that the cleanup cache key reads

Status: Observed (2026-10-30, found reading the code for decision 0448 stage 2)

Symptom: none seen in output. After decision 0352's strip, a native
build's cleanup cache (cln, decision 0382) key carried the default
settings: `fe_settings(c, ...)` in lower runs after `c = c.link_view(...)`,
and link_view built a fresh `Checker` with only the modules, diagnostics,
skip fields and parse cache. So `dwarf`, `no_preempt`, `det`, `race`,
`map_*`, `inline_str`, `pack_lists`, the `set_*` presets and the async
preemption flags all read as their defaults in the key: an `-s` build and
a default build of the same program keyed alike.

Cause: link_view copies what the link reads, and the cache key built in
the passes (after the strip) was a reader nobody listed.

Why nothing broke: each function's cln key also holds its SSA body and
the builder's own flags, and the settings change the bodies they matter
to. On main at 669af2667, a default build of bench/perf/http/xo followed
by `-s` and `--no-preempt` builds under `XO_BACKEND_CACHE=verify` passed
(verify fails a build whose hit differs from the passes' fresh output);
`--race` builds keep the front end (no `front_end_freed`) and were never
affected.

Fix: link_view copies the settings (compiler/check load.xo); the test
"link_view keeps the build's settings" in compiler/check check_test.xo
sets each one and reads it back from the view. Any new Checker setting
that the backend or a cache key reads after the lowering must be copied
there too.
