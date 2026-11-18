package arm64

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestPushRunGrowNoLoop: a push run's growth path (compiler/arm64
// pushrun.xo push_n) is straight line code, never a loop back to its
// capacity check. Each run was a loop (grow, then jump to the check
// that branched to grow), so a constant list literal of 220,000 pushes
// was as many loops, and the arm64 loop passes' per loop block sets
// took the compiler past 18 GB (task #329, std/backoff fuzz seed 19).
func TestPushRunGrowNoLoop(t *testing.T) {
	t.Parallel()
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	dir := filepath.Join("testdata", "pushgrow")
	args := append(append([]string{}, cli[1:]...), "asm", "--backend=arm64", "--target=darwin/arm64", dir)
	cmd := testtmp.Guard(exec.CommandContext(ctx, cli[0], args...))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	fn := constFunc(t, string(out), "ROWS")
	blocks, order := asmBlocks(fn)
	grows := 0
	for _, l := range order {
		if !strings.Contains(blocks[l].body, "_xo_x_obj_rl_grow_") {
			continue
		}
		grows++
		next := blocks[l].next
		if next == "" {
			continue
		}
		// the block the growth goes on to must not branch back to it
		if regexp.MustCompile(`\tb\.[a-z]+ ` + regexp.QuoteMeta(l) + `\n`).MatchString(blocks[next].body) {
			t.Errorf("growth block %s jumps to %s, which branches back to it:\n%s%s", l, next, blocks[l].body, blocks[next].body)
		}
	}
	if grows == 0 {
		t.Fatalf("no _xo_x_obj_rl_grow_ call in the constant's function:\n%s", fn)
	}
}

// constFunc returns the assembly of the function initializing the
// constant named name.
func constFunc(t *testing.T, asm, name string) string {
	t.Helper()
	i := strings.Index(asm, "// func xo.const[")
	for i >= 0 {
		line := asm[i:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		end := strings.Index(asm[i+1:], "\n// func ")
		body := asm[i:]
		if end >= 0 {
			body = asm[i : i+1+end]
		}
		if strings.Contains(line, "."+name+"]") {
			return body
		}
		if end < 0 {
			break
		}
		i = i + 1 + end + 1
	}
	t.Fatalf("no function for the constant %s in the assembly", name)
	return ""
}

type asmBlock struct {
	body string
	next string // the label an unconditional b at the end goes to, else the next label
}

var asmLabel = regexp.MustCompile(`^(L[A-Za-z0-9_]+):$`)

// asmBlocks splits a function's assembly at its labels.
func asmBlocks(fn string) (map[string]*asmBlock, []string) {
	blocks := map[string]*asmBlock{}
	var order []string
	cur := ""
	for _, line := range strings.Split(fn, "\n") {
		if m := asmLabel.FindStringSubmatch(line); m != nil {
			if cur != "" && blocks[cur].next == "" {
				blocks[cur].next = m[1]
			}
			cur = m[1]
			blocks[cur] = &asmBlock{}
			order = append(order, cur)
			continue
		}
		if cur == "" {
			continue
		}
		b := blocks[cur]
		if b.next != "" {
			continue
		}
		b.body += line + "\n"
		if f := strings.Fields(line); len(f) == 2 && f[0] == "b" {
			b.next = f[1]
		}
	}
	return blocks, order
}
