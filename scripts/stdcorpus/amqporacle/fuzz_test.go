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

// Two delivery annotations sections, the first with a symbol that is not
// UTF-8: go-amqp keeps the last section and never shows the first, but
// std/amqp decodes every section and refuses the symbol (its documented
// departure), so the case is one std/amqp refuses (TestAmqpFuzz seed 21 on
// darwin/arm64, 2026-11-17, task #152).
func TestFuzzBadUTF8InDroppedSection(t *testing.T) {
	b, err := hex.DecodeString("005371d10000001100000002a306782df09f7f8070ffffffff005371d10000001900000004a305782d780079a100a308782d68c3a96c6c6f512d005373d00000001e00000003980100000000000000000000000000000040a106e4b896e7958c005375a004626f6479")
	if err != nil {
		t.Fatal(err)
	}
	if got := fzDecode(b); got != "error" {
		t.Fatalf("fzDecode = %q, want error", got)
	}
}

// Every section twice after the header (delivery annotations, annotations,
// properties, data), the header a list32 whose size (115) runs past its five
// fields: go-amqp keeps the last properties, std/amqp refuses the second one
// (its documented departure), so the case is one std/amqp refuses
// (TestAmqpFuzz seed 104 on linux/arm64 host 3, 2026-11-18, task #152).
func TestFuzzSecondPropertiesAfterLongHeader(t *testing.T) {
	b, err := hex.DecodeString("005370d00000007300000005415006700000a697405202005371d10000000d00000002a305782d6b6579a000005372d10000000900000002550360a7ff005373d0000000160000000c4040404040a104636f727240404040405294005375a00863c985b1dff08ffd005371d10000000d00000002a305782d6b6579a000005372d10000000900000002550360a7ff005373d0000000160000000c4040404040a104636f727240404040405294005375a00863c985b1dff08ffd")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("sections %x at %v", fzSections(b), fzSectionAt(b))
	if got := fzDecode(b); got != "error" {
		t.Fatalf("fzDecode = %q, want error", got)
	}
}
