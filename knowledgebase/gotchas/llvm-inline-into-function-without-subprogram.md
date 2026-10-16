# LLVM: code inlined into a function without a DISubprogram loses its lines

Status: Observed (2026-10-16, LLVM backend line tables, decision 0239).

Symptom: llvm-symbolizer gives `??:0:0` for main's code, and the binary
has no subprogram for main.

Cause: LLVM inlined Xo's main into xo_entry (gen.xo), which had no
DISubprogram; locations of instructions inlined into a function without
one are dropped.

Fix: xo_entry gets a subprogram (main's file and first line) and !dbg on
its call and return. Any new generated LLVM function that calls Xo
functions with debug information needs the same.
