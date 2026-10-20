# Upstream ThreadSanitizer 21.1.8 crashed at start on macOS 27

Status: Observed (2026-10-20, Xo Lang 2, decision 0273 part 3).

Symptom: every program linked with the ThreadSanitizer runtime built from
LLVM compiler-rt 21.1.8 (scripts/race-runtime.sh's first pin) died with
SIGSEGV before main on macOS 27.0.1: lldb stopped in the runtime's
`__sanitizer::atomic_load` with a null pointer. A plain C race test
built by clang and pointed at that dylib crashed the same way, so it was
not Xo's code generation or linking. Apple's runtime from the Command
Line Tools (Apple clang 21) and the dylib built from LLVM 23.1.3 both run
the same programs and report the race.

Cause: the upstream runtime of an LLVM release older than the macOS
version does not know that version's process layout (the sanitizer
runtimes track each macOS release); Apple ships its clang with its own
patched copy. Not narrowed further.

Fix: pin a release newer than the macOS the seeds and releases are built
and run on (23.1.3; since LLVM 22 a release has only the whole
llvm-project source, from which race-runtime.sh extracts compiler-rt and
the cmake modules). Check a new pin with the C race test (a global
written by main and a thread: one report) and the race table
(internal/llvm TestRaceSyncTableArm64 with XO_RACE_RUNTIME set to the
built dylib) before packing it. A macOS release newer than the pin may
need the pin raised again.
