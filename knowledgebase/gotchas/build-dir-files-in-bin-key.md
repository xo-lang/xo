# A file the build leaves in its directory changes the binary cache key

Status: Observed (2026-10-30)

**Symptom.** internal/arm64 TestMachoBinCache failed on main: "the second
build added cache entries (4 to 5): not a hit". The two binary entries
had the same size and bytes but different keys.

**Cause.** The native link keys (compiler/xo cache.xo `macho_bin_key`,
`native_bin_key`, `go_bin_key`) hash every file of the build directory
(`key_tree`) except the subdirectories they skip. Decision 0418's spill
files (compiler/arm64 parallel.xo `spill_part`, 1dcb9065c) were written
as `spill-N.bin` in the build directory itself. A cold build makes them
(it has new per-function entries); a warm build whose entries all hit
makes none. So the cold build's link key named about 15 MB of spills
the warm build's did not, and an identical second build missed.

**Fix.** The spills go to the build directory's `spill/` subdirectory
(cache.xo `SPILL_DIR`), which every key_tree caller skips, emptied at
the start of each build. A new file the build writes into its directory
that is not a link input goes in a skipped subdirectory too, or the
binary cache stops hitting.

Since 2026-10-30 scripts/gate.sh runs internal/arm64's cache tests (`-run
'Cache|BinCache'`, TestMachoBinCache among them) after the named steps
whenever the branch changes compiler/xo's bcache.xo, parallel.xo,
packseg.xo or cache.xo (XO_GATE_CACHE=0 skips that): 1dcb9065c's gate ran
none of them.
