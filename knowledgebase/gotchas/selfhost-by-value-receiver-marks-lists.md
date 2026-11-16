# Xo (go backend): a by value method on a state struct makes its next list writes copy

Status: Observed (2026-10-11, the go build of compiler/llvm)

Symptom: the go build of the self hosted LLVM emitter took 12.8 s for
three lowerings of compiler/check (under load); a CPU profile
(`XO_CPUPROFILE`) showed `rt.List_copyOut` under `Gen.field` and most
of the rest in the garbage collector and `memclrNoHeapPointers`: whole
per value lists (`lazy`, `names`, `addr`) were copied on single element
writes, once per instruction, so the cost grew with the square of the
function's size.

Cause: the emitter's state is one struct (`Gen`) with `var` methods
that write its per value lists, and read only helpers declared with a
by value receiver (`fn (g: Gen) mem_only(...)`). Calling a by value
method passes the struct by value; on the go backend a struct holding
lists copied out of a place has its lists' blocks marked shared
(std/rt list.go, "Who sets shared"), and a block once shared copies
itself at the next element write through any header. So every read
only call between two writes made the next write copy the whole list.

Fix: declare the helpers `fn (var g: Gen)` too (compiler/llvm, 17
methods): 12.8 s to 4.2 s for the same three lowerings, the output
unchanged. In a self hosted pass whose state struct holds lists that
are written per element, give every method of the state a `var`
receiver, or pass the few fields a helper reads as arguments.

Observed again (Xo Lang 3, 2026-11-15, std/brotli's encoder): plain
functions taking the encoder state by value (`fn update_nodes(e: Enc,
...)`, `find_all_dict_matches(e: Enc, ...)`, 22 of them) between writes
to its hash tables made each position's write through `var e: Enc`
copy the whole binary tree (2 << lgwin ints). Quality 11 on a 34 KB
input took 69 s on the go backend and under 1 s on arm64 and llvm,
which borrow instead; with every Enc parameter `var`, 0.16 s and the
same bytes. The brotli corpus's encode tests on the go backend
(TestStdCorpora/go/brotli, 60 s per test) guard it. The same rule holds
for a std port: a state struct whose lists are written per element is
passed `var` everywhere, as the Go original passes a pointer.
