# Branch free code still fails dudect on an Apple core without DIT

Status: Observed (2026-11-19, std/native/llrt/tls12_test.xo's CBC
dudect on host 1, an M4 Pro running linux/arm64; task #384)

## Symptom

The TLS 1.2 CBC record check (mask based padding check, MAC hashed over
every block, MAC read by a mask over every offset) failed dudect at 10^6
measurements: one padding against another |t| 26 to 73, a bad MAC
against bad padding 54 to 79, while the A/A pairs passed. Timed alone,
no piece leaked more than 1 to 2 ns; the decryption followed by the MAC
read leaked |t| up to 278, and a plain copy followed by the same read did
not. Copying each record to a fresh buffer changed nothing, and the
class means differed in the tail (the sign of t changed between
percentile crops), not in the median.

## Cause

The code ran without Arm's DIT bit. Apple cores keep data dependent
timing (among it the data memory-dependent prefetcher) unless PSTATE.DIT
is set, so the timing followed the plaintext the hardware AES kernel had
just written, though no instruction branched or indexed on it. Decision
0619 sets DIT around std's other secret dependent operations; open_cbc
was not among them.

## Fix

Wrap the whole secret dependent operation in `ct.dit_on()` and
`ct.dit_off(old)` (tls12.xo open_cbc_at). Under DIT every pair passed
(decrypt and MAC read |t| 0.7 to 1.6). A new constant time operation on
secrets gets the same section and a test that it sees DIT set inside
(cpu.has_dit()); a dudect run on an arm64 host that has DIT (the Mac,
host 1) is the check, since Neoverse hosts without FEAT_DIT do not show
it.
