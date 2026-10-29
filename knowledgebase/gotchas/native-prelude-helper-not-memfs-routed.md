# A native prelude helper called from native prelude code is not routed to MemFs in tests

Status: Observed (2026-10-29, write_atomic)

Symptom: the prelude corpus (a test's MemFs) failed natively with
`cfg/.app.toml.000001: no such file or directory` from write_atomic's
sync, while the Go backend passed. After that, a second failure: the
test timed out at 1 minute.

Cause:
- compiler/ssa testmode.xo mem_fs_call routes calls to testing's
  memfs_* only at call sites that reach call.xo's std-module branch:
  methods of Fs, File and FileLock. Those dispatch on the value. A plain
  function of the native prelude (fs_sync_path) called from another
  native prelude function (atomic_fill) never reaches that branch, so it
  ran the OS path code on a MemFs path.
- The timeout: testing.xo memfile_sync called memfs_load (which takes
  the MemFs state from the test world) and returned without memfs_store.
  The next MemFs call then waited for the state forever.

Fix:
- Make such a helper go through Fs methods, or treat the OS answer that
  cannot happen for a real file as the MemFs case. write_atomic does the
  latter: not found right after a write is nothing to sync.
- Every memfs_load is paired with a memfs_store before any return,
  including an error.
