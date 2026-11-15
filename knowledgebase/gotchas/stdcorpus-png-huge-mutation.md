# A bare `stdcorpus -check` reached 46 GB: Go decoded a mutated PNG header

Status: Observed 2026-11-15 (lang3, while regenerating yamlfuzz and xmlcodec)

## Symptom

`go run ./scripts/stdcorpus -check` with no corpus named, under heavy.sh on
the Mac, was stopped by the memory reaper at 46 GB of the job's tree. Run one
corpus at a time and every generator stays under 210 MB except png:
14 GB resident, and `GODEBUG=gctrace=1` showed a 52 GB live heap.

## Cause

The png corpus mutates its seed files (scripts/stdcorpus/png.go pngMutate);
one mutation rewrote an IHDR byte and fixed the CRC, which made a header of
NRGBA64 59x117440542. Go's `png.Decode` allocates the whole image (about
55 GB) before it finds the data short, and writes the rows it decodes into it.
The generator ran Go's Decode on every case to record Go's result. stdcorpus
already runs the corpora one after another, so the one input was enough.

## Fix

`pngDecodeTooLarge` leaves out of DECODE every input whose header gives Go an
image over 4 GiB (two lines left the corpus; the seeded sequence is unchanged,
so no other case moved). The HUGE list still covers header-only dimension
overflow. png now peaks at 171 MB, and a bare `-check` fits a gate slot.

When a generator runs Go's decoder on mutated or fuzzed input, bound what the
input's header lets Go allocate before calling it.
