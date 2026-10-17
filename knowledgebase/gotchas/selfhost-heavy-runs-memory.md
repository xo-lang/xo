# One self hosted run can take 5 GB, and -short sampled one

Status: Observed (2026-10-12)

Symptom: `go test -short ./internal/selfhost` took 175 to 208 s on the
development Mac (the repository's `-short` budget is about 10 s per
package). Logging each Xo process's peak resident size
(runXoBatches, jobs_test.go) showed one process of the Xo SSA builder at
57.8 s and 4,738 MB and one of the Xo LLVM emitter at 28.7 s and
5,853 MB, both on the single run compiler/llvm.

Cause: the short tier took every 25th run of moduleDirs, and
compiler/llvm is one of them; it uses most of the other components
(compiler/check, compiler/ssa), so the binaries load and lower all of
them. The old batches of 60 runs hid it: the process holding
compiler/llvm was just the slow one. The same runs make the job count a
poor bound on memory: four heavy processes at once are about 20 GB.

Full tier (2026-10-12, c8g.8xlarge, 32 vCPU, 63 GB): with seven heavy
slots (one per 8 GiB) TestSelfhostArm64 ran seven Xo arm64 emitter
processes on self hosted components at once, 5.0 to 13.2 GB each, next
to a 10 GB test process; memory in use reached 59 of 63 GB, SSH timed
out, and the kernel killed the process on compiler/xo (`signal:
killed`). Each such run also takes 15 to 19 minutes.

Fix: the short tier of TestSelfhostSSA and TestSelfhostLLVM skips the
mergeHeavy runs (`-short` fell to 31 to 52 s). Every run of the package now
draws from two pools (internal/selfhost/jobs_test.go): XO_SELFHOST_JOBS
for every unit and XO_SELFHOST_HEAVY_JOBS (default one per 8 GiB of
memory, one per 16 GiB in the full tier) for builds and mergeHeavy
runs, which run one per process; scripts/remote-full.sh defaults to
r8g.8xlarge (32 vCPU, 256 GiB, the same spot price). When
a new component lands under compiler/, mergeHeavy already counts it;
check the peak RSS lines (`go test -v`) before raising the heavy count.

Two more traps found on the way (2026-10-12):

- On linux, a child's ru_maxrss counts the memory it shared with the
  parent before exec, so every Xo process of a 35 GB test process
  reported 35 GB. jobs_test.go prints n/a for a child whose value does
  not exceed the parent's own; use `ps` on the instance for real figures.
- The comparisons, not the compilers, were the critical path:
  canonClosures matched one regular expression alternating every closure
  name, which took 50.8 s on the build stage of compiler/xo alone and
  most of TestSelfhostSSA's 1,850 s subtests in the full tier. The
  renamer in ssa_test.go does the same in 1.4 s (TestRenamer checks it
  against the expression). Look at the `compare` and `Go ...` lines of
  `go test -v` before blaming the Xo binaries.
