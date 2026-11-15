# A negative codegen pattern on an intrinsic's name matches a call to its software body

Status: Observed 2026-11-14 (Xo Lang 2)

## Symptom

`xo dev codegen` failed on main after 8264da41a (std/crc32's short
input path): `test/codegen/intrinsics amd64 main.xo:100: flate.crc32:
amd64 code matches -"crc32x"`. Nothing was wrong with the code.

## Cause

The amd64 check was `-"crc32x"`, meant as "no IEEE crc32 instruction on
amd64". On amd64 `cpu.crc32x` is not lowered to an instruction: it is a
call to its Xo body, the software bit loop. 8264da41a made flate.crc_tail
inline into flate.crc32, so its arm64 only path (behind hw()'s
HW_CRC32, never set on amd64) now held `callq _xo_f_cpu_2ecrc32x`,
which the pattern matched by name.

## Fix

Write a negative pattern against the instruction's text, not a function
name that can appear in a call: `-"crc32[bwlq] %"` (8fc7d5716). To tell
the two apart, read the listing: `xo asm --backend=amd64 --std=STD DIR`
and look at the matching line.
