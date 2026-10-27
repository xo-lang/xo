package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// x86Needs lists the cpu intrinsics the amd64 backend lowers to
// instructions beyond x86-64's SSE2 baseline (Go's GOAMD64=v1), with the
// cpu.hw() bit a caller must check first (gotchas/amd64-baseline-sse2.md,
// 2026-10-27: the SSSE3 ChaCha20 kernel ran with no check).
var x86Needs = map[string]string{
	"aes_round":         "HW_AES",
	"aes_last":          "HW_AES",
	"aes_enc10":         "HW_AES",
	"aes_enc12":         "HW_AES",
	"aes_enc14":         "HW_AES",
	"clmul_lo":          "HW_CLMUL",
	"clmul_hi":          "HW_CLMUL",
	"sha256_block":      "HW_SHA256",
	"fma_hw":            "HW_FMA",
	"chacha_blocks4":    "HW_SSSE3",
	"chacha_stream4":    "HW_SSSE3",
	"chacha_poly_seal4": "HW_SSSE3",
	"chacha_poly_open4": "HW_SSSE3",
	"chacha_poly_seal8": "HW_AVX2",
	"chacha_poly_open8": "HW_AVX2",
	"aes_gcm8_enc10":    "HW_AES",
	"aes_gcm8_enc12":    "HW_AES",
	"aes_gcm8_enc14":    "HW_AES",
	"aes_gcm8_dec10":    "HW_AES",
	"aes_gcm8_dec12":    "HW_AES",
	"aes_gcm8_dec14":    "HW_AES",
}

// x86GateHelpers are the functions that call such an intrinsic without a
// check of their own because every caller checks: file:function and the
// check.
var x86GateHelpers = map[string]string{
	"aesgcm.xo:hw_n":        "encrypt_n and encrypt4 check c.hw & HW_AES",
	"aesgcm.xo:enc_at":      "its callers ctr_at (from ctr_o), ctr_at_a (from seal_into_at) and tag_fin check HW_AES",
	"aesgcm.xo:mul_hw":      "mul checks g.hw & HW_CLMUL",
	"aesgcm.xo:ghash_hw":    "ghash_o checks HW_CLMUL",
	"aesgcm.xo:sub_word_hw": "schedule calls it only under aes, its hw & HW_AES",
	"aesgcm.xo:gcm_bulk":    "every caller checks g.fused() (HW_AES and HW_CLMUL), seal_fused's callers seal_into and seal_into_aad too",
}

var x86Call = regexp.MustCompile(`\bcpu\.([a-z0-9_]+)\(`)
var xoFunc = regexp.MustCompile(`^\s*(?:pub\s+)?fn\s+(?:\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)`)

// TestX86SIMDGated fails when std calls an intrinsic amd64 lowers to an
// instruction past SSE2 from a function that neither checks the cpu.hw()
// bit it needs nor is a listed helper whose callers do: on a CPU without
// the instruction it would fault with SIGILL. cpu.xo itself (the bodies
// and its test entry points) is not checked.
func TestX86SIMDGated(t *testing.T) {
	var bad []string
	err := filepath.WalkDir(filepath.Join(root, "std"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".xo") || strings.HasSuffix(p, "_test.xo") || filepath.Base(p) == "cpu.xo" {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		lines := strings.Split(string(src), "\n")
		fnStart, fnName := 0, ""
		for i, line := range lines {
			if m := xoFunc.FindStringSubmatch(line); m != nil {
				fnStart, fnName = i, m[1]
			}
			for _, m := range x86Call.FindAllStringSubmatch(line, -1) {
				need, ok := x86Needs[m[1]]
				if !ok {
					continue
				}
				body := strings.Join(lines[fnStart:i+1], "\n")
				if strings.Contains(body, need) || strings.Contains(body, "target.arch != TargetArch.Amd64") && strings.Contains(body, "HW_") {
					continue
				}
				if _, ok := x86GateHelpers[filepath.Base(p)+":"+fnName]; ok {
					continue
				}
				bad = append(bad, rel+":"+itoa(i+1)+": "+fnName+" calls cpu."+m[1]+" with no "+need+" check")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(bad)
	for _, b := range bad {
		t.Error(b)
	}
}
