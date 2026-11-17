package main

import (
	"encoding/hex"
	"testing"
)

// stdproven fuzz protobuf seed 3 (2026-11-17): the sunmsg input's field 8
// float 0xffa18e1c is a signaling NaN, which the oracle's dynamicpb wrote back
// quieted (0xffe18e1c) while std/protobuf kept its bits.
func TestProtoQuietFl(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"0806451c8ea1ff7b7c10db9fc8e4c5054344", "0806451c8ee1ff7b7c10db9fc8e4c5054344"},
		{"451c8ee1ff", "451c8ee1ff"},                             // a quiet NaN stays
		{"450000803f", "450000803f"},                             // 1.0 stays
		{"2d1c8ea1ff", "2d1c8ea1ff"},                             // field 5 (fixed32) keeps any bits
		{"0a05451c8ea1ff451c8ea1ff", "0a05451c8ea1ff451c8ee1ff"}, // bytes of a length field are skipped
		{"45ff", "45ff"},                                         // cut short
	} {
		in, _ := hex.DecodeString(c.in)
		got := hex.EncodeToString(protoQuietFl(in))
		if got != c.want {
			t.Errorf("protoQuietFl(%s) = %s, want %s", c.in, got, c.want)
		}
		if hex.EncodeToString(in) != c.in {
			t.Errorf("protoQuietFl(%s) changed its input", c.in)
		}
	}
}
