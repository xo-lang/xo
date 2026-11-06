# std twin cbor: decode and marshal rows (2026-11-06)

Host 2 (linux/arm64, 18 CPUs), `xo bench --ab`, n=5, Xo at b4d8abfaa (branch cborenum). Ratio is Xo wall over Go wall (fxamacker/cbor).

| workload | Go ms | Xo ms | ratio | interval | RSS ratio | verdict |
|---|---|---|---|---|---|---|
| udecode | 10.83 | 10.84 | 1.001 | 0.962-1.04 | 0.146 | within noise |
| umarshal | 3.828 | 11.23 | 2.934 | 2.857-3.065 | 0.184 | worse |

Decode matches Go. Typed marshal (`derive Cbor`) is 2.9x slower: the next step is XO_PROF on umarshal (the Value tree built before encoding is the likely cost; a direct writer like derive Proto's would avoid it).
