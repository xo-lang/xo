package gen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReplayGoCallback runs a program whose output depends on how Go
// callbacks on goroutines of Go's (sync.WaitGroup.Go) interleave with
// main (decision 0073): the same seed prints the same order every time,
// distinct seeds give distinct orders, and a recording logs each
// callback's entry and replays to the same output.
func TestReplayGoCallback(t *testing.T) {
	replayBuilds(t, func(t *testing.T, race bool) {
		bin := buildReplayProg(t, filepath.Join("testdata", "replay", "gocallback"), race)
		dir := t.TempDir()
		orders := map[string]bool{}
		for seed := 1; seed <= 4; seed++ {
			first := runProg(t, bin, dir, "", fmt.Sprintf("XO_SEED=%d", seed))
			if first.code != 0 {
				t.Fatalf("seed %d: exit %d\n%s", seed, first.code, first.stderr)
			}
			orders[first.stdout] = true
			for i := 0; i < 5; i++ {
				again := runProg(t, bin, dir, "", fmt.Sprintf("XO_SEED=%d", seed))
				if again.stdout != first.stdout {
					t.Fatalf("seed %d run %d printed another order:\n%s--- first\n%s", seed, i+2, again.stdout, first.stdout)
				}
			}
		}
		if len(orders) < 2 {
			t.Errorf("4 seeds gave only %d orders", len(orders))
		}
		for i := 0; i < 4; i++ {
			trace := filepath.Join(dir, fmt.Sprintf("run%d.xotrace", i))
			recd := runProg(t, bin, dir, "", "XO_RECORD="+trace)
			if recd.code != 0 {
				t.Fatalf("recording %d: exit %d\n%s", i, recd.code, recd.stderr)
			}
			data, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			if n := bytes.Count(data, []byte(`"op":"go.callback"`)); n != 4 {
				t.Fatalf("the trace has %d callback entries, want 4:\n%s", n, data)
			}
			rep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+trace)
			if rep.code != 0 || rep.stdout != recd.stdout || !strings.Contains(rep.stderr, "xo replay: identical run") {
				t.Fatalf("replay %d: exit %d\n--- stdout\n%s--- recorded\n%s--- stderr\n%s", i, rep.code, rep.stdout, recd.stdout, rep.stderr)
			}
		}
	})
}
