# read_mapped through a wrapped Fs reads instead of mapping

Status: Observed (2026-10-30, ADR 0402's first user)

**Symptom.** `read_mapped(os.fs, path)` in xo's link returned the right
bytes but the link's memory budget still grew by the whole store
(`phase link.read_store ... heap 90 MB`, from 56), and the edit build
was no faster: the store was read, not mapped. The same call in a small
program mapped (2.7 MB peak RSS for a 30 MB file, against 32.7 MB read).

**Cause.** `xo build` wraps `os.fs` in recorder.xo's RecFs (the run
cache's input record, decision 0360) and cachestats.xo's Fs: user values
converted to Fs (decision 0307). read_mapped is a free function, so a
user Fs cannot answer it; it calls `fs.read(path)`, which copies (and a
RecFs hashes what it records).

**Fix.** Map through the process's own Fs: link.xo Ctx.real_fs, set
before the wrappers (main.xo), which the link uses for std's store (a
path under the cache, which neither wrapper records). Any other caller
that maps a large file under a wrapped Fs needs the same, and checks a
phase's heap figure (the memory budget does not count a mapping) or the
peak RSS to see that it maps.
