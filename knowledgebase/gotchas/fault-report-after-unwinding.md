# A fault's report is printed after the unwinding released its values

Status: Observed (2026-10-18, decision 0268, xo-4)

Symptom: with XO_TRACEBACK=locals reading the frames' variables when the
report was printed, a List in main's x24 (lldb: count 1, length 3,
items 10, 20, 30) showed as `List[Int] (len 48)`: its memory held a Str
object whose bytes were a path ("...c/res/lo...").

Cause: fault.xo exit prints the report after the unwinding has run every
frame's landing pad, which released the frames' references; the list was
freed, and the report's own DWARF reading (the dSYM's path) reused the
memory. The registers and the DWARF locations were right (lldb agreed).

Fix: make the text at the fault, before the unwinding (fault.xo trace
calls cabi.xo fault_traceback with now 1, through its address so the
raise's frame stays small), keep it in the trace block, and print that
at exit. A fault while it is made does not recurse (the block is marked
-1 while it runs). Anything else read about a faulting program's values
for its report has to be read at the fault too.
