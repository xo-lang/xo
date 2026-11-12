# A dudect pool that alternates the classes slot by slot leaks its layout

Status: Observed (2026-11-12, std/native/llrt/rsa_test.xo's RSA unpad
dudect; decision 0619's measurements)

## Symptom

OAEP decoding failed dudect on linux/arm64 (native, DIT on) and on
linux/amd64 (LLVM -O2) at 10^6 measurements, in every pair including
the A/A pair (two sets of the same class, |t| up to 22.9), with a fixed
message length and a clean control (a piece that reads no block passed,
max 1.5). Timed alone, only the pieces that read the whole block (MGF1's
SHA-256, the zero scans) failed; those that read a few bytes passed. The
x86 code of pkcs1_unpad was branch free.

## Cause

The harness put both classes' blocks in one buffer at KK byte steps,
class A in the even slots and class B in the odd ones (so a block's
place in memory would not differ by class), and measured a pair by
picking a random slot of each class; the A/A pair is built the same
way. With KK = 128 and a buffer base that is not 128 aligned, the one
block per 4096 byte page that crosses into the next page is every 32nd
slot: always the same parity, so always the same class. A check that
reads the whole block pays the page crossing (a second TLB entry,
another prefetch stream) on that class only, which dudect sees as a
timing difference between the classes, and between two sets of one
class.

## Fix

Lay the classes out in alternating runs of 4096 / KK slots (rsa_test.xo
dudect): each run of 32 slots holds one page's worth of a class, so the
crossing blocks split evenly between the classes whatever the base.
The same runs with the balanced layout passed everywhere (arm64 mgf1
A/A 3.1, the whole unpad 2.4; amd64 LLVM -O2 2.1), and the old layout,
in the same run, failed as before. Any dudect pool shared by classes
must place them so that every address period (cache line, page) holds
both classes equally, and a run's A/A pair is the check that this
holds.
