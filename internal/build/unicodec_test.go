package build

import (
	"slices"
	"testing"
)

// TestIOFlagsTextC checks that code calling the C path of the number and
// time text compiles the runtime with -DXO_TEXT_C (decision 0193), and
// that the default (Xo) path, which calls rs_push_word and the other
// primitives, does not.
func TestIOFlagsTextC(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_rs_append_duration\n", true},
		{"call ptr @xo_rs_append_quote(ptr %1, i64 %2, ptr %3, i64 %4, ptr %5)\n", true},
		{"bl _xo_int_parse_ok\n", true},
		{"bl _xo_time_parse\n", true},
		// a string literal naming it (xo1's own io_flags table) is no call
		{"\t.ascii \"xo_int_parse\"\n", false},
		{"@s = private constant [12 x i8] c\"xo_int_parse\"\n", false},
		{"bl _xo_rs_push_word\nbl _xo_rs_append_bytes\nbl _xo_bytes_span\nbl _xo_int_wrap\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_TEXT_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_TEXT_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsStep5 checks the C paths of decision 0193 step 5: the leaf
// text of Debug (XO_LLRT_C=show), the report of a property test's case
// (report), and the zone rules (tz) compile their C only into programs
// that call it; the primitives the Xo paths call (xo_tz_data,
// xo_gen_choices, xo_rs_append_rune) turn on nothing.
func TestIOFlagsStep5(t *testing.T) {
	for _, c := range []struct {
		code, flag string
		want       bool
	}{
		{"bl _xo_rs_append_u64\n", "-DXO_SHOW_C", true},
		{"call ptr @xo_rs_append_rune_quote(ptr %1, i64 %2)\n", "-DXO_SHOW_C", true},
		{"bl _xo_rs_append_rune\nbl _xo_rs_push_word\n", "-DXO_SHOW_C", false},
		{"bl _xo_prop_report\n", "-DXO_REPORT_C", true},
		{"bl _xo_gen_choices\nbl _xo_test_write\nbl _xo_prop_best\n", "-DXO_REPORT_C", false},
		{"bl _xo_tz_load_host\n", "-DXO_TZ_C", true},
		{"bl _xo_tz_offset\n", "-DXO_TZ_C", true},
		{"bl _xo_tz_data\nbl _xo_tz_find\nbl _xo_tz_keep\nbl _xo_tz_now\nbl _xo_tz_local_name\n", "-DXO_TZ_C", false},
		{".ascii \"xo_tz_offset\"\n", "-DXO_TZ_C", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), c.flag)
		if got != c.want {
			t.Errorf("IOFlags(%q) has %s: %v, want %v", c.code, c.flag, got, c.want)
		}
	}
}

// TestIOFlagsInts checks that code calling the C path of the integer
// operations (XO_LLRT_C=ints, decision 0195) compiles the runtime with
// -DXO_INTS_C, as does a call of an intrinsic's C version.
func TestIOFlagsInts(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_int_op\n", true},
		{"call i64 @xo_int_conv(i64 %1, i64 8, i64 0, i64 8, i64 1, ptr %2, i64 9)\n", true},
		{"bl _xo_rune_of\n", true},
		{"bl _xo_int_in_range\n", true},
		{"bl _xo_rune_valid\n", true},
		{"bl _xo_floor_div\n", true},
		{"bl _xo_str_hash\n", true},
		{"bl _xo_int_wrap\n", true},
		{"call i64 @xo_int_fits(i64 0, i64 %1, i64 %2)\n", true},
		{"bl _xo_fault_raise\nbl _xo_rs_push_word\n", false},
		{".ascii \"xo_int_op\"\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_INTS_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_INTS_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsRC checks the C path of the reference counts (XO_LLRT_C=rc,
// decision 0207 stage 2): -DXO_RC_C only when rc_c.xo's calls are in
// the code, not for the exports every program defines.
func TestIOFlagsRC(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_rc_inc_c\n", true},
		{"call i64 @xo_rc_claim_c(i64 %1)\n", true},
		{"bl _xo_rc_dec_shared_c\n", true},
		{"_xo_x_rc_inc:\n\tb _xo_rc_inc\nbl _xo_rc_free\n", false},
		{"define i64 @xo_x_rc_dec(i64 %p)\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_RC_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_RC_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsAlloc checks the C path of the heap (XO_LLRT_C=alloc,
// decision 0207 stage 3): -DXO_ALLOC_C only when alloc_c.xo's calls are
// in the code, not for the heap_* exports every program defines nor for
// the platform calls of alloc.xo (xo_heap_*).
func TestIOFlagsAlloc(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_alloc_new_c\n", true},
		{"call void @xo_alloc_free_mt_c(i64 %1, i64 %2)\n", true},
		{"_xo_x_heap_alloc:\n\tb _xo_f_cabi_2eheap__alloc\nbl _xo_heap_chunk\n", false},
		{"bl _xo_alloc\nbl _xo_heap_large\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_ALLOC_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_ALLOC_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsDigestAndGlob checks the C paths of std/crypto's hashes
// (XO_LLRT_C=digest) and MemFs.fail_next's match (glob, decision 0195):
// compiled only into programs that call them; crypto.equal
// and Ed25519, which stay C, turn on nothing.
func TestIOFlagsDigestAndGlob(t *testing.T) {
	for _, c := range []struct {
		code, flag string
		want       bool
	}{
		{"bl _xo_crypto_sha256\n", "-DXO_DIGEST_C", true},
		{"call ptr @xo_crypto_pbkdf2(ptr %1, i64 %2)\n", "-DXO_DIGEST_C", true},
		{"bl _xo_crypto_argon2id\n", "-DXO_DIGEST_C", true},
		{"bl _xo_crypto_verify_password\n", "-DXO_DIGEST_C", true},
		{"bl _xo_crypto_b64\n", "-DXO_DIGEST_C", true},
		{"bl _xo_crypto_equal\nbl _xo_crypto_ed25519_sign\n", "-DXO_DIGEST_C", false},
		{"bl _xo_test_glob\n", "-DXO_GLOB_C", true},
		{"bl _xo_gen_any_int\n", "-DXO_GEN_C", true},
		{"bl _xo_gen_sized\n", "-DXO_GEN_C", true},
		{"call ptr @xo_gen_str(i64 %1, ptr %2)\n", "-DXO_GEN_C", true},
		{"bl _xo_gen_intn\nbl _xo_gen_draw\nbl _xo_gen_depth\nbl _xo_gen_enter\nbl _xo_gen_deep\n", "-DXO_GEN_C", false},
		{"bl _xo_test_memfs_take\n", "-DXO_GLOB_C", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), c.flag)
		if got != c.want {
			t.Errorf("IOFlags(%q) has %s: %v, want %v", c.code, c.flag, got, c.want)
		}
	}
}

// TestIOFlagsDeletedCPaths checks that the runtime symbols of the C
// paths decision 0198 deleted (unicode, strings, num, jsontext) turn on
// no flag: their -DXO_*_C sections are gone from the runtime.
func TestIOFlagsDeletedCPaths(t *testing.T) {
	for _, code := range []string{
		"bl _xo_rune_to_case\nbl _xo_str_lower\nbl _xo_unicode_ranges\n",
		"bl _xo_str_trim\nbl _xo_str_replace\n",
		"bl _xo_float_parse_ok\nbl _xo_b64_decode\nbl _xo_bytes_hex_ok\n",
		"bl _xo_json_parse\nbl _xo_rs_append_json_float\nbl _xo_rs_append_b64_quoted\n",
	} {
		if got := IOFlags([]byte(code)); len(got) != 0 {
			t.Errorf("IOFlags(%q) = %v, want none", code, got)
		}
	}
}

// TestIOFlagsFspathC checks that code calling the C path of the Fs path
// policy compiles the runtime with -DXO_FSPATH_C (decision 0193 step 6),
// and that the default (Xo) path, which calls the raw system calls,
// compiles xo_fs.c (-DXO_FS) without it.
func TestIOFlagsFspathC(t *testing.T) {
	for _, c := range []struct {
		code       string
		fspath, fs bool
	}{
		{"bl _xo_fs_mkdir_all\n", true, true},
		{"call i64 @xo_fs_remove_all(ptr %1, i64 %2, ptr %3, i64 %4)\n", true, true},
		{"bl _xo_fs_create_temp\n", true, true},
		{"bl _xo_fs_cwd\n", true, true},
		{"\t.ascii \"xo_fs_cwd\"\n", false, false},
		{"bl _xo_fs_raw_mkdir\nbl _xo_fs_raw_kind\nbl _xo_fs_raw_fail\n", false, true},
		{"call ptr @xo_fs_raw_getcwd(ptr %1)\n", false, true},
	} {
		flags := IOFlags([]byte(c.code))
		if got := slices.Contains(flags, "-DXO_FSPATH_C"); got != c.fspath {
			t.Errorf("IOFlags(%q) has -DXO_FSPATH_C: %v, want %v", c.code, got, c.fspath)
		}
		if got := slices.Contains(flags, "-DXO_FS"); got != c.fs {
			t.Errorf("IOFlags(%q) has -DXO_FS: %v, want %v", c.code, got, c.fs)
		}
	}
}

// TestIOFlagsAtomicC checks that code calling the C path of Atomic's
// operations compiles the runtime with -DXO_ATOMIC_C (decision 0200), and
// that the default path, which allocates with xo_atomic_new and does the
// rest inline or in Xo, does not.
func TestIOFlagsAtomicC(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_atomic_get\n", true},
		{"call void @xo_atomic_set(ptr %1, i64 %2)\n", true},
		{"bl _xo_atomic_add\n", true},
		{"bl xo_atomic_max\n", true},
		{"bl _xo_atomic_new\n\tldar x0, [x9]\n\tcasal x10, x0, [x9]\n", false},
		{"call ptr @xo_atomic_new(i64 0)\n%t1 = load atomic i64, ptr %t0 seq_cst, align 8\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_ATOMIC_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_ATOMIC_C: %v, want %v", c.code, got, c.want)
		}
	}
}

// TestIOFlagsSched checks the C path of the scheduler core
// (XO_LLRT_C=sched, decision 0212): -DXO_SCHED_C only when sched_c.xo's
// calls are in the code, not for the scheduler's exports, the hooks, or
// the emitted switch every program defines.
func TestIOFlagsSched(t *testing.T) {
	for _, c := range []struct {
		code string
		want bool
	}{
		{"bl _xo_sched_c\n", true},
		{"call i64 @xo_sched_c(i64 0, i64 %1, i64 %2, i64 0)\n", true},
		{"_xo_x_rq_put:\n\tb _xo_f_cabi_2erq__put\nbl _xo_sched_hook\nbl _xo_sched_run_task\n", false},
		{"bl _xo_stack_switch\nbl _xo_tlocal_get\n", false},
	} {
		got := slices.Contains(IOFlags([]byte(c.code)), "-DXO_SCHED_C")
		if got != c.want {
			t.Errorf("IOFlags(%q) has -DXO_SCHED_C: %v, want %v", c.code, got, c.want)
		}
	}
}
