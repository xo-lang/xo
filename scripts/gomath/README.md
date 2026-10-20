# scripts/gomath

Regenerates std/rt/gomath_gen.go (decision 0287): Go's math package with
its assembly removed and every float64 product wrapped in `float64(...)`,
which forbids the Go compiler's fused multiply-add, so the Go backend's
math gives the native backends' bits on every architecture.

```
mkdir -p /tmp/pm && cp $(go env GOROOT)/src/math/*.go /tmp/pm/
cd /tmp/pm && rm -f *_test.go *_asm.go *_s390x.go exp_amd64.go
sed -i '' '/^\/\/go:build/d' *_noasm.go log_stub.go stubs.go
sed -i '' 's/^package math$/package pm/' *.go
# unsafe.go: replace the unsafe casts with math.Float64bits and friends
go run ./scripts/gomath /tmp/pm std/rt/gomath_gen.go   # from the repository root
```

Then add `"math"` to the generated imports and run std/native/llrt's
gomath_test (Go's results, generated from the same sources as a
linux/amd64 GOAMD64=v1 program, which does not fuse).
