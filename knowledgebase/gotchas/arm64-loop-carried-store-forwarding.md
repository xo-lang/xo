# A loop that stores a value and loads it back next iteration is slow

Status: Observed (2026-10-08, internal/arm64, bench sieve)

Symptom: the native sieve ran at 1.5x of the LLVM backend although its
push loop had about the same instruction count, and shortening the inner
loop made no difference.

Cause: `list.push` loaded the list length from the header, stored the
new length, and the next iteration loaded it again. The length was a
loop carried dependency through memory: each iteration waited for store
to load forwarding (several cycles) instead of a register add. LLVM keeps
the length in a register for the whole loop (scalar promotion) and only
stores it.

Fix: the backend gives every `List` block parameter a companion register
holding its length, set on every incoming edge from what the predecessor
knows (or one load), and records lengths a push or pop computes; per
block memos then never reload a length the code already has
(`internal/arm64/opt.go`, `lenParam` in `isel.go`). Dead code removal
drops the companion when nothing reads it. Sieve went from 1.5x to 1.1x
of LLVM. The same shape (a value written to memory and read back in the
next iteration) is the first thing to look for when a native loop is
slower than its instruction count suggests.
