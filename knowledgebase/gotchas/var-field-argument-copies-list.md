# A struct's list field passed as a `var` argument is copied on the callee's first write

Status: Observed (2026-11-20, task #271, std/brotli store_range_h5)

Symptom: `store_range_h5(e.buckets, e.h_num, ...)`, both `var`
parameters, gave the right output and ran bench/perf/brotli enc6 at
three times its instructions (1.99 G against 0.71 G, the Mac,
`/usr/bin/time -l`) and five times its cycles.

Cause: a `var` argument is copied in and out (spec 5.2): the callee gets
the list's value and the caller stores the result back through the
place. While the call runs the struct still holds the list too, so it
has two owners and the callee's first write takes the copy-on-write
path: all 2 MB of the hash table, once a call.

Fix: take the field out of the struct first, so the local is the only
owner, and put it back after:

```
var bk = e.buckets
e.buckets = []
fill(bk, ...)        // `var bk` in the callee: written in place
e.buckets = bk
```

`[]` is the shared empty list, so the two stores cost nothing. If the
callee faults, the struct is left with the empty field; std/brotli's
encoder is discarded then. Check a new `var` list parameter fed from a
field with an instruction count (callgrind, or `time -l` on the Mac)
before timing anything. Task #470 (parked) is the compiler side: lower
such an argument as a move when nothing else can reach the struct
during the call.
