# std's objects named the working directory of whoever started xo dev stdcache

Status: Observed (2026-10-30)

**Symptom.** Two warm edit builds of the same program with the same xo, from
fresh caches, gave binaries that differed in 3,600 bytes of .debug_str: one
held std's build directory (`CACHE/std/.build-HASH/obj/.stdobj-prog`) in each
std object's comp_dir slot (DW_SLOT, 1024 bytes each), the other held ".".
Started from a deleted directory, `xo dev stdcache` failed (`xo dev stdobj
exit 1: xo: no main function in the root module`) and the build fell back
to the whole program.

**Cause.** `xo dev stdobj` ran its two child builds of the empty program in
the caller's working directory, with only PWD set to the program. A build's
compilation directory is the working directory's line root (main.xo
gm.comp_dir: abs_line_root of cwd_path), so std's store, and every binary
linked from it, carried whatever directory the build that started the
std cache ran in: the cache's own directory gave the temporary path, any
other gave ".", and a deleted one failed the child's checks.

**Fix.** stdobj_run runs each child in the program's directory (Cmd.dir), so
std's comp_dir slots no longer name a temporary path and std's store is the
same bytes whatever the caller's cwd: on the arm64 host, stdcache started
from the cache's parent, from another directory and from a deleted one
gave one store (sha256 c1be5d069168bd58...), the deleted one no longer
failing. ab-host.sh's edit mode
(89cc1a476) ran stdcache on both sides from /work/abNAME.out, the cache's
parent, so both sides carried the temporary path alike and neither an --ab
nor an identity check there could show it; xo-lang-6's xl88 runs it from
the program copy. Lesson: a determinism control must vary the working directory and wipe every cache
(and HOME) between runs; one script that `rm -rf`'d its own cwd found this.
