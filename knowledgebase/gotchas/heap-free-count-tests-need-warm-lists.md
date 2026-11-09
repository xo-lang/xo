# A heap free count test needs warm free lists

Status: Observed (2026-11-09, Mac Studio)

**Symptom.** `TestStdModules/arm64/native/prelude` failed 5 of 5 on the
Mac Studio with `list_swap: a Str swap changes no reference count`
(`expect failed: after == before`), at commits that passed on the Mac
mini the night before.

**Cause.** The test compares `heapos.nfree` summed over the size classes
before and after making, swapping and dropping two heap strings. Each
string is freed exactly once, as it should be, but when its size class
has no free block the allocator carves a fresh one from the heap, and
its free then adds one to the total (before 5, after 7). Whether the
classes had free blocks depended on what ran earlier in the process, so
the result differed by machine.

**Fix.** Run the measured operation once before taking the first count,
so the measured run takes its blocks from the free lists and returns
them (std/native/prelude_test.xo). A test that counts free blocks must
warm the classes it uses first, or count allocations and frees instead.
