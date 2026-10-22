# Decision 0330 step 3 measured: slices are views (2026-10-22)

Instructions retired (`/usr/bin/time -l`, native arm64, Mac), the same
program built with the xo and std before and after the step (the commit
before it and the step's commit). Table values are the median of 3 runs
with the empty-run setup (about 14.3 M) left in; the ratio is of totals.

| program | work | before | after | change |
|---|---|---|---|---|
| mime parse_media_type | 200,000 calls | 1,798 M | 1,338 M | -25.6% |
| multipart.parse | 100,000 calls | 4,857 M | 4,076 M | -16.1% |
| time Str layout format | 600,000 calls | 6,214 M | 6,400 M | +3.0% |
| time Layout format | 600,000 calls | 2,388 M | 2,766 M | +15.8% |
| time Str layout parse | 600,000 calls | 3,303 M | 3,261 M | -1.3% |
| time Layout parse | 600,000 calls | 1,600 M | 1,654 M | +3.4% |

Per call: mime 8,993 to 6,694 instructions, multipart 48,566 to 40,756
(the 8,620 and 47,970 baselines of decision 0330 were measured on a
slightly different build).

Slicing no longer allocates, so mime and multipart (token slices) gain
16 to 26%. time does not gain: it indexes bytes (`b[i]`) instead of slicing,
and every byte index and length now pays the offset add and the length mask
that owned strings did not (done once per block per value), which is the
likely cause of the +16% on the Layout format walk (not yet profiled).
Step 4 targets this (hoist the data
pointer and masked length of a loop invariant string out of the loop; a
local that was never sliced is a known off 0 value) and then the ports that
slice (time's field names, url, header parsing).
