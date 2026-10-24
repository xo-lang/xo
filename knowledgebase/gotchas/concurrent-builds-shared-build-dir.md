# Two builds of one program wrote the same part objects

Status: Observed and fixed (2026-10-24)

## Symptom

`xo: link: .../build/<16 hex>/arm64/prog-darwin.1.o: not a 64 bit Mach-O file`
in one of two concurrent `xo build` of the same program (cmd/xo
TestBuildStagedOutputs, once in many runs; an editor build and a CLI build of
one program can do the same). It passed alone.

## Cause

The build directory is keyed by the program (build_dir), so concurrent builds
shared `<build dir>/arm64`, and each wrote its assembly and part objects there
with `os.fs.write` (truncate, then write: not atomic) and linked them from
there. One truncating `prog-darwin.1.o` while the other's linker read it gave a
partial file. Only the final output was staged. The backend cache was not
involved: its writes are temporary name plus rename (cache_put). Renaming the
parts alone would not do: two builds with other flags could link part 0 of one
and part 1 of the other.

## Fix

Each build writes its files in `<build dir>/arm64/w-<random>` and removes it
after the links (main.xo); the program of a build without `-o` stays in
`<build dir>/arm64/bin`. `XO_KEEP_ASM=1` keeps the files in `<build dir>/arm64`
as before. A build that fails leaves its `w-*` directory for the build
directory's 30 minute prune. A cross build that stops at the assembly (asm_only)
writes it in `<build dir>/arm64` itself, where its message names it (test
TestBuildStopsAtAssemblyKeepsIt). Test: cmd/xo TestBuildWorkDirsPrivate.
