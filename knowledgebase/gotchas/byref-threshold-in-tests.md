# A test struct just under 256 bytes tests none of the by reference code

Status: Observed (2026-10-11, internal/arm64 testdata/run/byref_var)

Symptom: the first version of the golden program for decision 0151
passed on arm64 and the Go backend, but its assembly showed `State`
passed as words through the stack: the program never reached the by
reference convention it was written for.

Cause: the convention starts at 256 bytes (`byRefMin`, 32 words on
arm64). `State` had 31 words (Str fields are two words, Lists one), so
it was 248 bytes. The LLVM sizes differ again (a Str is a 16 byte
struct, Bool one byte), so one struct can be by reference on one
backend and not on the other.

Fix: count the words of every struct a by reference test relies on
(internal/arm64 layout.go) and keep a margin (byref_var's `State` has
33 words); check the assembly once (`str` and `ldr` through `x0` rather
than stack slots at the call). Changes of `byRefMin` or of the layout
need the goldens' structs checked again.
