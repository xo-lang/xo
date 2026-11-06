# std twin protobuf: unmarshal and marshal rows (2026-11-06)

Host 2 (linux/arm64, 18 CPUs, OFF_EXCLUSIVE=1), `xo bench --ab`, n=5, Xo 0.40.0 at 1dac451746 (branch proto). Ratio is Xo wall over Go wall (google.golang.org/protobuf v1.36.12 with protoc-gen-go output). Workload: a 12,123-byte message; checksums agree (go and xo: 213123 and 12135123, tw-pb.sh).

| workload | Go ms | Xo ms | ratio | interval | RSS ratio | verdict |
|---|---|---|---|---|---|---|
| unmarshal | 36.12 | 16.58 | 0.459 | 0.458-0.464 | 0.119 | better |
| marshal | 17.52 | 77.23 | 4.409 | 3.686-4.541 | 0.122 | worse |

Unmarshal is about 2.2x faster than Go. Marshal is 4.4x slower: the next step is XO_PROF on marshal (the size pass, per field BytesBuilder copies and nested message size recomputation are the likely cost; Go computes sizes once and writes into one buffer).
