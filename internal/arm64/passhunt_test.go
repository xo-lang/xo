package arm64

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// passHuntTargets names, for each pass_* golden, the optional passes its
// program aims at (2026-11-02, the targeted pass hunt: each program was
// written against what a pass assumes; slp_alias_store found slp_f64
// sinking a store pair past a load of the same element through another
// index). The x86 passes are run by the amd64 goldens of the full tier.
var passHuntTargets = map[string][]string{
	"pass_alias_idx":         {"cluster_mem", "reuse_loads", "slp_f64", "cse", "copy_prop"},
	"pass_alias_loop":        {"hoist_fields", "hoist_invariants", "reuse_loads", "inplace_elems", "cse", "cluster_mem"},
	"pass_big_loop_rc":       {"rc_thunk_loops", "rc_thunk", "sink_retains", "cancel_rc_pairs"},
	"pass_cold_pads":         {"tail_merge_cold", "cold_saves", "mark_tails", "chain_pads", "pad_release_order"},
	"pass_const_edges":       {"fold_consts", "jump_tables", "thread_const", "select_compares", "hoist_consts"},
	"pass_fault_latch":       {"tail_dup", "thread_switch", "jump_tables", "tail_merge_cold", "thread"},
	"pass_flags_carried":     {"thread_cset", "select_compares", "reuse_flags", "thread_const", "thread", "copy_prop"},
	"pass_hoist_alias":       {"hoist_invariants", "hoist_fields", "cse", "reuse_loads", "inplace_elems", "sink_retains"},
	"pass_hollow_reach":      {"hollow", "devirtualize", "specialize_args"},
	"pass_inplace_alias":     {"inplace_elems", "cluster_mem", "reuse_loads", "slot_group"},
	"pass_latch_swaps":       {"tail_dup", "thread", "copy_prop", "thread_cset", "place_latches"},
	"pass_loops_ranges":      {"lsr", "ranges", "unroll_small", "push_runs", "place_latches"},
	"pass_map_share":         {"map_core", "specialize_args"},
	"pass_rc_cancel":         {"cancel_rc_pairs", "sink_retains"},
	"pass_rc_pairs":          {"cancel_rc_pairs", "sink_retains", "rc_thunk"},
	"pass_select_cmp":        {"select_compares", "thread_cset", "reuse_flags", "thread"},
	"pass_slp_pairs":         {"slp_f64", "reuse_loads", "cluster_mem"},
	"pass_spill_swaps":       {"shrink_split", "slot_group", "arg_slots", "cold_saves", "copy_prop", "hoist_consts"},
	"pass_stack_bb":          {"stack_small"},
	"pass_stack_esc":         {"stack_small"},
	"pass_stack_uuid":        {"stack_small"},
	"pass_struct_spills":     {"slot_group", "arg_slots", "cluster_mem", "reuse_loads", "stack_small", "fold_tuples"},
	"pass_switch_carried":    {"thread_switch", "jump_tables", "tail_dup", "thread", "thread_cset"},
	"pass_uniq_spec":         {"uniq_var_params", "specialize_args", "devirtualize", "inplace_elems"},
	"pass_view_join":         {"view_lens", "copy_prop"},
	"pass_view_len":          {"view_lens", "copy_prop"},
	"pass_x86_words":         {},
	"slp_alias_store":        {"slp_f64", "reuse_loads", "cluster_mem"},
	"pass_take_in_place":     {"take_in_place", "inplace_elems"},
	"pass_take_in_place_ref": {"take_in_place"},
	"rc_empty_entry":         {"cancel_rc_pairs", "sink_retains"},
	"rc_empty_inplace":       {"inplace_elems", "take_in_place"},
	"slp_alias_54":           {"slp_f64"},
	"slp_alias_64":           {"slp_f64"},
	"slp_alias_66":           {"slp_f64"},
	"slp_alias_387":          {"slp_f64"},
	"slp_build_swapped":      {"slp_f64"},
}

// TestGoldenPassesOff runs the pass hunt's goldens with every optional pass
// off and with each pass the program aims at off alone: each must print
// its want.txt (which the LLVM and Go backends print too), so a pass that
// changes what a program does shows here by name, beside the default
// build's TestGoldenRun.
func TestGoldenPassesOff(t *testing.T) {
	t.Parallel()
	if !native() {
		t.Skip("the host is not arm64")
	}
	if testing.Short() {
		t.Skip("builds and runs programs")
	}
	src, err := os.ReadFile(filepath.Join("..", "..", "compiler", "arm64", "passes.xo"))
	if err != nil {
		t.Fatal(err)
	}
	var all []string
	for _, m := range regexp.MustCompile(`(?m)^\s+"([a-z0-9_]+)",$`).FindAllStringSubmatch(string(src), -1) {
		all = append(all, "-"+m[1])
	}
	if len(all) < 20 {
		t.Fatalf("read %d passes from passes.xo; its list's form changed", len(all))
	}
	for name, targets := range passHuntTargets {
		dir := filepath.Join("..", "llvm", "testdata", "golden", name)
		want, err := os.ReadFile(filepath.Join(dir, "want.txt"))
		if err != nil {
			t.Fatal(err)
		}
		sets := map[string]string{"all": strings.Join(all, ",")}
		for _, p := range targets {
			sets[p] = "-" + p
		}
		for label, passes := range sets {
			name, dir, label, passes := name, dir, label, passes
			t.Run(name+"/"+label, func(t *testing.T) {
				t.Parallel()
				got, err := buildRunWith(t, dir, xotool.Options{Backend: xotool.BackendArm64, Env: []string{"XO_PASSES=" + passes, "XO_BACKEND_CACHE=off"}})
				if err != nil {
					t.Fatalf("build: %v", err)
				}
				if string(want) != got.String() {
					t.Errorf("output differs from want.txt with XO_PASSES=%s\n--- want\n%s\n--- got\n%s", passes, want, got)
				}
			})
		}
	}
}
