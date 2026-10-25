# heapos.nfree is not a balance oracle when strings are allocated inside the window

Status: Observed (2026-10-25)

Symptom: a test that read `heapos.nfree` over all 73 classes before and
after a helper that built two `List[Str]` of `"alpha" + "!"` style
strings reported different totals, with no swap involved and no wrong
value: the control rows (two literal-only lists, one literal-only list)
balanced, the heap-string rows did not (0374 step 3a's
`rt.list_swap2` counting row in std/native/prelude_test.xo, before it
was rewritten).

Cause: the heap's per-class free counts move on their own when objects
of a class are allocated and freed inside the measured window; the
allocator does not return every block to the same class free list it
came from, and the totals are not conserved by an allocate/free pair.
Only the single-list, single helper shape that 0374 step 1 used happened
to balance.

Fix: measure a count, not an allocator total. Read the object's own
reference count from its first word (`rt.slot_load(rt.obj_addr(s), 0, 0)`,
the read `llrt/obj.xo`'s private `count` does) before and after the
operation, and compare element addresses for identity. One more trap in
the same row: `rt.obj_addr(xs[i])` holds a temporary reference to the
element, so a count read after one sees 2 and not the swap's 1; read the
counts first, then check identities.
