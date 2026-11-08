# A front end cache entry reads a type the replaying build has not made yet

Status: Observed (2026-11-08, `xo check` of bench/perf/http with std's record)

Symptom: every warm `xo check` of bench/perf/http printed `xo: warning:
the front end cache entry ... (closure Store.list_json.fn9 ...: its
entry missed) did not replay ...; building again without it`, and
XO_FE_STATS=1 showed `fe dropped miss.read 1`.

Cause: compiler/ssa makes a sized integer type (Int8 to UInt64, Rune)
and Float32 at its first use (Module.int_type, float32). A body's entry
lists the types it found already made as reads, and fe_replay missed
when one was absent. The check that recorded the entry had made UInt8
before the closure; the next check (std's declarations from their
record, decision 0428's declaration parse) had not, so the replay
missed every time and the entry was stored again.

Fix: fe_replay makes a sized integer or Float32 it reads (compiler/ssa
fecache.xo). A read of any other type that can be made from its name
alone would need the same; one that needs the checker cannot.
