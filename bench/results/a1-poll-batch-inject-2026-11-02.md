# a1: a poll's batch past its first fiber injected (decision 0324, 2026-11-02)

Xo Lang 4. The owner chose P (poll batches only) as the Linux default
on 2026-11-02; R (reader grants) is parked. Base for the experiment rows:
4a16f4d80; P 4361a2ba2, R e21aedbce. Landed branch: l4-p.

## bench/perf/http (CRUD server, warm-up, 5 rounds, medians)

| host | change | c32 | c256 | c1024 |
|---|---|---|---|---|
| linux/arm64 (first host, exclusive) | P | req/s -0.8%, CPU a request -5.9% (ranges apart), p999 -3.6% | level, p999 -9.5% | req/s +2.7% (apart) |
| linux/arm64 | R | req/s +4.1%, CPU -4.3% (apart), p999 +5.2% | level | level |
| linux/amd64 (offload host, exclusive) | P | req/s -3.4%, CPU +0.6% | | +5.4% |
| linux/amd64 | R | req/s +10.9%, CPU -9.9% (apart) | +8.2% | +9.0%, p999 -9.1% |
| windows/amd64 (pinned) | P | +9% | | worse, load generator errors on both sides |
| windows/amd64 (pinned) | R | level | level | -17%, errors on both sides |

## xo bench --ab (linux/arm64)

| change | program | result |
|---|---|---|
| P | bench/perf/httpstreamsrv | CPU 0.886, RSS 0.94 |
| R | bench/perf/httpstreamsrv | wall level, CPU 1.006 (amd64: RSS 1.117) |
| P, R | bench/perf/conc/xo/rwmutex, 15 rounds | level (1.006, 1.009): the bench runs about 6 ms with CPU equal to wall, no parallel readers |
| P, landed branch against its parent | bench/perf/httpstreamsrv | CPU 0.893 (0.883 to 0.913), wall 0.992, RSS 0.96 within noise (a144a3115 against 7e5f1c0d1, 3 rounds, host load 8 on both sides) |

## Gate (default change)

linux/arm64 second host, d424e0c5b: internal/arm64 TestGoldenRun (134 s),
internal/stdtest (1312 s), bootstrap fixed point (s1 72 s, s2 126 s,
s3 119 s): pass.

Raw outputs: ~/xo-lead/notes/xo-lang-4/out/a1p*.txt, a1r*.txt, pgate.txt
(the agent's notes, not in the repository).
