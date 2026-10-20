package arm64

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestAmd64PassesFinish: the x86_64 MIR passes finish on a program with
// two loads read by one instruction (2026-10-20: x86_sink_loads moved
// such loads past each other forever, and every amd64 build of rsa hung;
// -short and the fixed point build arm64 only, so nothing else ran the
// pass before a Docker run).
func TestAmd64PassesFinish(t *testing.T) {
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	dir := filepath.Join("testdata", "x86sink")
	args := append(append([]string{}, cli[1:]...), "asm", "--backend=amd64", "--target=linux/amd64", dir)
	cmd := exec.CommandContext(ctx, cli[0], args...)
	cmd.Env = append(os.Environ(), "XO_NOLIBC=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		t.Fatalf("xo %s: did not finish in 180 s", strings.Join(args, " "))
	}
	if err != nil {
		t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	if !strings.Contains(string(out), "sum2") {
		t.Fatalf("no sum2 in the assembly")
	}
}
