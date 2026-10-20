# Front end cache verify compared the wrong field after a header change

Status: Observed (2026-10-20, fixed the same day)

Symptom: none at the time. XO_BACKEND_CACHE=verify passed every front
end cache entry built by one xo, whatever its SSA, from 7a1807c4 until
the fix. Found by reading fe_same while extending the entry format.

Cause: the entry header is read by position. FE_CACHE_VERSION v8 added
the recorder (the xo that wrote the entry) after the own flags in
fe_entry, fe_replay and fe_applies, but fe_same, verify's comparison,
still read two strings and then the tail, so it compared the recorder
(equal within one xo) where the tail belongs.

Fix: fe_same skips the recorder. compiler/ssa fecache_test.xo checks
that the recorder is not compared and the flags and tail are. Rule:
every reader of an entry (fe_replay, fe_applies, fe_same) changes in
the same commit as the writer (fe_entry), and a header change adds a
unit test that reads the new header.
