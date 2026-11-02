# List floor of one slot for big elements (decision 0452 addendum), 2026-11-01

A new list whose elements are over 256 bytes has room for 1, not 4; the first
growth of any list goes to 4 (so growing from 1 costs one extra copy at most).

Mac, tlssrv with 300 TLS connections, bytes in use (XO_PROF=heap): tlssrv.save
2,200 kB to 560 kB (-75%), total 3,430 kB to 1,853 kB (-46%): save's one-element
List[Conn] took the 8,184 byte class (cap 4 of a 1,872 byte Conn).

linux/arm64 `--ab` (host 1, base dd732be75, new 16f0680a8, 5 rounds; self 3):

| workload | wall B/A | verdict |
|---|---|---|
| alloc big1 (300,000 lists of one 320 B struct) | 1.27 (0.998 to 1.315) | noise (B spread 16%) |
| alloc bign (60,000 lists growing to 1..20 big) | 1.046 (0.991 to 1.05) | noise |
| alloc mixed | 0.993 | noise |
| alloc small | 1.047 (1.003 to 1.062) | worse, at the host's noise edge (first run, 1.009: noise) |
| self build | 0.994, max RSS 0.979 | better (machine loaded) |

Plain doubling from 1 measured bign 1.165 (1.157 to 1.195, 50e2a3900); jumping
the first growth to 4 removed it. RSS of the alloc programs is identical (7020 kB).
Gate (host 2): internal/arm64, selfhost wasm (merge tier), fixed point, internal/llvm
with XO_GATE_LLVM=all and Go parity, all ok; amd64 sample ok.
