package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// inlineNotLeaf are the rt.* intrinsics compiler/arm64 calls.xo lowers
// that inline.xo's inline_intrinsic leaves out, each with its reason: a
// function calling one is not a leaf for the inliner's cost. Leaving a
// name out only costs speed; listing one whose lowering calls (a bl, an
// svc, a retain) would make a leaf of a function that calls.
var inlineNotLeaf = map[string]string{
	"rt.str_bytes": "--inline-str (decision 0370): calls the runtime for an inline Str", "rt.str_inline": "--inline-str (decision 0370): a tag test and a branch",
	"rt.aes_ctr10": "block kernel", "rt.aes_ctr12": "block kernel", "rt.aes_ctr14": "block kernel",
	"rt.aes_enc10": "block kernel", "rt.aes_enc12": "block kernel", "rt.aes_enc14": "block kernel",
	"rt.aes_enc10x4": "block kernel", "rt.aes_enc12x4": "block kernel", "rt.aes_enc14x4": "block kernel",
	"rt.aes_gcm8_dec10": "block kernel", "rt.aes_gcm8_dec12": "block kernel", "rt.aes_gcm8_dec14": "block kernel",
	"rt.aes_gcm8_enc10": "block kernel", "rt.aes_gcm8_enc12": "block kernel", "rt.aes_gcm8_enc14": "block kernel",
	"rt.aes_gcm_dec10": "block kernel", "rt.aes_gcm_dec12": "block kernel", "rt.aes_gcm_dec14": "block kernel",
	"rt.aes_gcm_enc10": "block kernel", "rt.aes_gcm_enc12": "block kernel", "rt.aes_gcm_enc14": "block kernel",
	"rt.aes_last": "block kernel", "rt.aes_round": "block kernel",
	"rt.chacha_blocks4": "block kernel", "rt.chacha_stream4": "block kernel",
	"rt.ghash4": "block kernel", "rt.ghash_blocks": "block kernel",
	"rt.mlkem_inverse_ntt": "block kernel", "rt.mlkem_mul_acc": "block kernel", "rt.mlkem_ntt": "block kernel",
	"rt.sha256_block": "block kernel",
	"rt.call_c2":      "a C call", "rt.call_c4": "a C call",
	"rt.sys6": "a system call", "rt.sys_clone": "a system call", "rt.sys_restorer": "a system call's address",
	"rt.sys_words": "a system call",
	"rt.mem_cmp":   "memcmp", "rt.mem_move": "memmove", "rt.mem_set": "memset",
	"rt.obj_at": "a retain", "rt.str_view": "a retain",
	"rt.unwind_resume": "the unwinder", "rt.unwind_save": "the unwinder",
	"rt.stack_switch": "a stack switch", "rt.fiber_start_addr": "a fiber's entry",
	"rt.frame_addr": "the frame pointer chain",
	"rt.cpuid1_ecx": "cpuid", "rt.cpuid7_ebx": "cpuid", "rt.xgetbv0": "xgetbv",
	"rt.ticks": "the clock's counter", "rt.ticks_hz": "the clock's counter",
	"rt.tlocal_addr": "thread local storage", "rt.tlocal_get": "thread local storage", "rt.tlocal_set": "thread local storage",
}

// TestInlineIntrinsicsListed fails when an rt.* intrinsic calls.xo lowers
// is neither in inline_intrinsic (instructions only: a function calling
// it may be a leaf) nor in inlineNotLeaf with its reason, when a name is
// in both, or when inlineNotLeaf names one calls.xo no longer lowers.
// rt.bytes_word and 22 others were lowered to instructions but missing
// from inline_intrinsic, so unicode in_ranges counted as a non-leaf and
// paid for its constants (DS-1, 2026-10-24).
func TestInlineIntrinsicsListed(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	arm := regexp.MustCompile(`(?m)^\s*("rt\.\w+"(?:\s*\|\s*"rt\.\w+")*)\s*=>`)
	name := regexp.MustCompile(`"(rt\.\w+)"`)
	lowered := map[string]bool{}
	for _, m := range arm.FindAllStringSubmatch(read("compiler/arm64/calls.xo"), -1) {
		for _, n := range name.FindAllStringSubmatch(m[1], -1) {
			lowered[n[1]] = true
		}
	}
	if len(lowered) < 50 {
		t.Fatalf("only %d lowered intrinsics in calls.xo: the match arm pattern no longer matches", len(lowered))
	}
	inl := read("compiler/arm64/inline.xo")
	i := strings.Index(inl, "fn inline_intrinsic(")
	if i < 0 {
		t.Fatal("no inline_intrinsic in compiler/arm64/inline.xo")
	}
	body := inl[i:]
	body = body[:strings.Index(body, "\n}")]
	leaf := map[string]bool{}
	for _, n := range name.FindAllStringSubmatch(body, -1) {
		leaf[n[1]] = true
	}
	var missing, both, stale []string
	for n := range lowered {
		_, nl := inlineNotLeaf[n]
		if leaf[n] && nl {
			both = append(both, n)
		}
		if !leaf[n] && !nl {
			missing = append(missing, n)
		}
	}
	for n := range inlineNotLeaf {
		if !lowered[n] {
			stale = append(stale, n)
		}
	}
	sort.Strings(missing)
	sort.Strings(both)
	sort.Strings(stale)
	if len(missing) > 0 {
		t.Errorf("lowered by calls.xo but in neither inline_intrinsic nor inlineNotLeaf (add each where it belongs): %s", strings.Join(missing, " "))
	}
	if len(both) > 0 {
		t.Errorf("in both inline_intrinsic and inlineNotLeaf: %s", strings.Join(both, " "))
	}
	if len(stale) > 0 {
		t.Errorf("in inlineNotLeaf but no longer lowered by calls.xo: %s", strings.Join(stale, " "))
	}
}
