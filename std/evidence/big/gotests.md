kind: gotests
module: big
result: pass
commit: 3a3845296
source: 02cb457e2e87cd95bd608354755d2414c8f5e6c0fdfc4aebd022c4b7cfd6b713
date: 2026-11-19
host: darwin/arm64
command: go run ./scripts/stdproven gotests -backends arm64,llvm -host "darwin/arm64" big
measure: 677 cases from 27 tables of Go's tests (go1.26.11), 2 backends (arm64,llvm), 0 mismatches with Go, 118 elements skipped (inputs not literals)

Go test table evidence (task #357): the inputs of these tables of Go's own tests, each run through Go (std/evidence/big/gotests/go) and the module (std/evidence/big/gotests/main.xo), every result equal, error text included.

- math/big int_test.go quoTests (quo): 2 cases, 0 skipped
- math/big int_test.go bitLenTests (bitlen): 12 cases, 0 skipped
- math/big int_test.go expTests (exp): 38 cases, 0 skipped
- math/big int_test.go gcdTests (gcd): 13 cases, 0 skipped
- math/big int_test.go rshTests (rsh): 24 cases, 0 skipped
- math/big int_test.go lshTests (lsh): 19 cases, 0 skipped
- math/big int_test.go bitwiseTests (bitwise): 17 cases, 0 skipped
- math/big int_test.go tzbTests (tzb): 7 cases, 0 skipped
- math/big int_test.go notTests (not): 6 cases, 0 skipped
- math/big int_test.go modInverseTests (modinv): 5 cases, 0 skipped
- math/big rat_test.go ratCmpTests (ratcmp): 9 cases, 0 skipped
- math/big rat_test.go ratBinTests (ratbin): 18 cases, 0 skipped
- math/big intconv_test.go stringTests (intstr): 43 cases, 15 skipped
- math/big ratconv_test.go setStringTests (ratset): 36 cases, 0 skipped
- math/big ratconv_test.go floatStringTests (floatstr): 17 cases, 0 skipped
- math/big float_test.go TestFloatMinPrec.range (fminprec): 11 cases, 0 skipped
- math/big float_test.go TestFloatSign.range (fsign): 6 cases, 0 skipped
- math/big float_test.go TestFloatIsInt.range (fisint): 14 cases, 0 skipped
- math/big float_test.go TestFloatInt64.range (fint64): 25 cases, 0 skipped
- math/big float_test.go TestFloatFloat64.range (ffloat64): 49 cases, 0 skipped
- math/big float_test.go TestFloatInt.range (fint): 17 cases, 0 skipped
- math/big float_test.go TestFloatRat.range (frat): 14 cases, 0 skipped
- math/big float_test.go TestFloatMantExp.range (fmantexp): 9 cases, 0 skipped
- math/big sqrt_test.go TestFloatSqrt.range (fsqrt): 12 cases, 0 skipped
- math/big floatconv_test.go TestFloatSetFloat64String.range (fset64): 117 cases, 0 skipped
- math/big floatconv_test.go TestFloatText.range (ftext): 120 cases, 0 skipped
- math/big floatconv_test.go TestFloatText.range (ftextr): 17 cases, 103 skipped

- arm64: 677 cases, 0 mismatches, 10ms
- llvm: 677 cases, 0 mismatches, 8ms
