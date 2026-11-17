package main

import (
	"encoding/hex"
	"testing"
)

// A header whose ttl is a ulong past what time.Duration holds in
// milliseconds (0xccc016197260 ms): go-amqp's MessageHeader.TTL wraps, so the
// case is one go-amqp cannot represent, not a std/amqp mismatch (TestAmqpFuzz
// seed 9 on darwin/arm64, 2026-11-17, task #152).
func TestFuzzHeaderTTLPastDuration(t *testing.T) {
	b, err := hex.DecodeString("005370d00000000f00000005425000800000ccc016197260415204005371d10000002500000004a306782df09f9880609874a303782d61d00000000c0000000260d2e0e003015601005372d10000000400000000005374d10000000b00000002a10072bfc0000000537772bfc00000")
	if err != nil {
		t.Fatal(err)
	}
	if got := fzDecode(b); got != "unsupported" {
		t.Fatalf("fzDecode = %q, want unsupported", got)
	}
	// a ttl that fits stays compared
	ok, _ := hex.DecodeString("005370d00000000f000000054250008000000000000003e8415204")
	if got := fzDecode(ok); got == "unsupported" || got == "error" {
		t.Fatalf("fzDecode(ttl 1000) = %q, want a decoded message", got)
	}
}
