# Lazy body parsing (decision 0351 stage 2 (b)): identity and --ab (2026-10-23)

XO_LAZY_PARSE=on parses the bodies of std and runtime files only when the
checker needs one (compiler/check load.xo `parse_lazy_path`, a sentinel
for an unforced body). Measured on the development Mac (Apple M4 Pro,
24 GB), xo from the branch rebased on main c9876fe44, under load from
other agents' jobs (loads in each table).

## Identity

Every 10th golden of internal/llvm/testdata/golden plus compiler/xo,
built with XO_LAZY_PARSE=off and =on to the same output name (the name
is embedded in the binary, so a comparison must keep it), with
XO_BACKEND_CACHE, XO_CHK_CACHE and XO_SHARED_PACK off:

    compared 33: identical 33, differing 0, exit mismatch 0

Determinism control: a hello built twice with lazy parsing off, to the
same name, is identical too; built to two different names (off1, off2)
the binaries differ, with identical disassembly. An earlier run of the
same day reported every program differing; its script did not survive,
and a difference in output name alone produces that result, so it is the
likely cause (not shown).

## --ab (A = XO_LAZY_PARSE=off, B = on; `xo bench --ab $XO -- build`)

| program, caches | rounds, load | wall B/A (95%) | cpu B/A (95%) | peak RSS B/A (95%) |
|---|---|---|---|---|
| bench/perf/http/xo, default (warm) | 10, 12.5 to 13.6 | 0.974 (0.88 to 1.124) noise | 0.886 (0.809 to 0.959) better | 0.589 (0.586 to 0.594) better |
| bench/perf/http/xo, check and fe caches off | 6, 15.5 to 19.4 | 1.262 (0.765 to 1.599) noise | 1.021 (0.953 to 1.186) noise | 0.835 (0.827 to 0.885) better |
| compiler/xo, default (warm) | 10, 14.6 to 27.6 | 0.929 (0.819 to 1.048) noise | 1.017 (0.936 to 1.044) noise | 0.931 (0.91 to 1.051) noise |
| compiler/xo, check and fe caches off | 6, 9.2 to 13.7 | 0.994 (0.989 to 1.012) noise | 1.023 (0.99 to 1.053) noise | 0.997 (0.972 to 1.03) noise |

Medians: the small server's warm build peaked at 927,512 KB (A) against
546,592 KB (B), cpu 1,755 against 1,555 ms.

## Reading

- No output change.
- A small program's warm build keeps 41% less memory at its peak and
  takes 11% less CPU: most std bodies it never needs are no longer
  parsed. Wall time is inside the noise of a loaded machine.
- compiler/xo uses most of std, so it parses nearly every body anyway:
  no measurable change.
