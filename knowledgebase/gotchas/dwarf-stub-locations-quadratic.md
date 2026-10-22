# Location lists grow as variables times fault stubs

Status: Observed (2026-10-22)

Symptom: a generated function of 24,000 lines (12,000 `let`s, each an
overflow checked add, all live across calls: a frame of about 96 KiB)
took over 10 minutes and 11 to 15 GB of memory to build for amd64, and
its linux/amd64 assembly passed 4 GiB, where the location list text's
join came out cut (gotchas/str-over-4gib-truncated.md): "assembler:
.Lxo_dwllend_... is used but not defined". internal/arm64's huge_frame
(4,500 values) built to a 268 MB executable.

Cause: compiler/arm64 dwvars.xo dv_field gives every variable an entry
for every fault stub of its function (decision 0268's locals: the
places the faulting frame keeps, sp moved by the stub's saves), so the
entries number variables times stubs, about 144 million there, each
three label differences and an expression in the assembly.

Fix: dv_field joins the entries of a run of stubs placed one after
another (DvStub.slot, the stub's index in g.stubs, consecutive) whose
place is the same: one entry from the first stub's start to the last
one's end. A variable in a spill slot is then one entry over the run of
stubs. A function whose variables still need more than 2,000,000 entries
(dwvars.xo DV_MAX_ENTRIES, about 100 MB of assembly) fails its build
with an error naming -s, instead of growing: before the join, a rerun
of the repro with --asm reached 37 GB on the 24 GB Mac and was stopped
by the owner. Run such repros under a memory watchdog (macOS ignores
`ulimit -v`; heavy.sh's HEAVY_MAX_GB stops a job past its cap).

Measured (2026-10-22, the Mac, under heavy.sh): huge_frame for linux/amd64
268 MB and 15.3 GB at the build's peak before, 3.3 MB, 1.08 GB and 17 s
after; the 24,000 line function for linux/amd64 failed after 11 GB
before, builds after (5.6 MB, 3.0 GB, 256 s; darwin/arm64 4.7 MB, 2.4 GB,
139 s). Its build with -s takes 10 s, so its debug information still
costs about two minutes elsewhere (open).
