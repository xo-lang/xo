# A push run across a fault point needs elements that own nothing

Status: Observed (2026-10-23; reverted 962042c53, see "Self build")

**Symptom.** Extending push runs (compiler/arm64 pushrun.xo) across a
landing split broke arm64 goldens. multipart_basic failed with "v490
in main.run used before it is defined", and archive_formats and
http_serve_content reported allocations still live at exit.

**Cause.** A push run stores all its values at its last push. Across a
value that may fault, the earlier pushes have not stored anything when
the fault unwinds. For elements with a reference count (a struct of
lists) the moved value is then neither in the list nor released, so
it leaks. Relaxing "the list's only use is the next push" to allow
releases also admitted runs where a pad block that releases the list
is selected before the push's block, and that pad read a value isel
had not made yet.

**Fix.** A run crosses a landing split only for Int, Float and Bool
elements, and the releases must be in blocks isel selects after the
push. A run within one block keeps its first rule (no other use). A
skipped push that a pad releases reads the run root's list. Golden
push_runs_fault; json scan's three words a token is the case it is
for.

**Self build (2026-10-23).** The fixed rule passed its gate: arm64
goldens, TestAmd64CrossBuilds, the Docker sample, and an xo built with
it. It still broke main (ce738f7c7, reverted 962042c53). An xo built
WITH the change failed to compile the compiler: "ssa/maplog.xo:1142:
in ssa.Builder.mlog_record: arm64 backend: v7399 used before it is
defined". The gate's xobuild compiles the new compiler with the
earlier xo, so code the new backend produces for the compiler's own
functions never ran. The cache kept the bad build
(checkout/c7fda1cc7695d8dc, from 09830074a). Later xobuilds tried it
as their seed, failed, and fell back to the prebuilt seed. That entry
is deleted.

**Rule.** A backend change to how ordinary code is compiled (isel,
MIR passes, the register allocator, inlining) runs a self build in its
gate: scripts/bootstrap-xo.sh s2, the new xo compiling compiler/xo.
Golden tests and cross builds do not cover the compiler's own
functions, and the 2-hourly fixed point comes after the bad build is
in the shared cache.
