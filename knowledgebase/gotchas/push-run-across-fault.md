# A push run across a fault point needs elements that own nothing

Status: Observed (2026-10-23)

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
