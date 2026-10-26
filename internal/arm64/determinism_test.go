package arm64

import (
	"path/filepath"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestAsmIgnoresOwnAllocation requires xo's output not to depend on how
// xo itself allocates: the same programs' assembly with XO_KEEP_FRONT=1
// (decision 0352: the checker and the SSA modules kept alive to the end,
// so the heap is reused differently after lowering), with the heap
// sampler on (XO_PROF=heap: sampled blocks change sizes and addresses
// from the first phase), and with neither must be the same text. A builder order taken from addresses, or from a map's
// order where it is not insertion order, would show here (asked for by
// the lead, 2026-10-23, after the front end cache's replay of a thunk
// depended on which body made it first).
func TestAsmIgnoresOwnAllocation(t *testing.T) {
	if testing.Short() {
		t.Skip("emits several programs twice")
	}
	golden := filepath.Join("..", "llvm", "testdata", "golden")
	off := []string{"XO_BACKEND_CACHE=off", "XO_BIN_CACHE=off", "XO_FE_CACHE=off"}
	for _, name := range []string{"dce_values", "closure_env_frame", "map_update_inplace", "view_lens", "nested_update"} {
		dir := filepath.Join(golden, name)
		for _, goos := range []string{"darwin", "linux"} {
			// each program and target in a subtest of its own, in parallel
			// (lead, 2026-10-26: 30 asm runs one after another)
			t.Run(name+"/"+goos, func(t *testing.T) {
				t.Parallel()
				kept, err := xotool.AsmEnv(dir, xotool.BackendArm64, goos+"/arm64", append(off, "XO_KEEP_FRONT=1"))
				if err != nil {
					t.Fatalf("%s %s kept: %v", name, goos, err)
				}
				freed, err := xotool.AsmEnv(dir, xotool.BackendArm64, goos+"/arm64", off)
				if err != nil {
					t.Fatalf("%s %s freed: %v", name, goos, err)
				}
				if string(kept) != string(freed) {
					t.Errorf("%s %s: the assembly differs with XO_KEEP_FRONT=1 (%d bytes against %d)", name, goos, len(kept), len(freed))
				}
				// the heap sampler from the start: sampled blocks take another
				// size and address, so every phase's layout differs
				prof := filepath.Join(t.TempDir(), "p.xoraw")
				sampled, err := xotool.AsmEnv(dir, xotool.BackendArm64, goos+"/arm64", append(off, "XO_PROF=heap", "XO_PROF_HEAP_RATE=4096", "XO_PROF_QUIET=1", "XO_PROF_OUT="+prof))
				if err != nil {
					t.Fatalf("%s %s sampled: %v", name, goos, err)
				}
				if string(sampled) != string(freed) {
					t.Errorf("%s %s: the assembly differs with the heap sampler on (%d bytes against %d)", name, goos, len(sampled), len(freed))
				}
			})
		}
	}
}
