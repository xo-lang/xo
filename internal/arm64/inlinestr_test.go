package arm64

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestInlineStrPutInPlace: under --inline-str (decision 0370) a
// BytesBuilder.put_le stores into the builder in place. It made the
// builder an object as "" + cur, which copied the whole builder on every
// put (gotchas/inline-str-put-copies-builder.md): a million puts into a
// 4 MB builder copied about 4 TB and never finished. Also a builder short
// enough to be inline, which put makes an object first.
func TestInlineStrPutInPlace(t *testing.T) {
	t.Parallel()
	cli, err := xotool.Command()
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	src := `fn main(os: Os) uses (stdio) {
  var b = BytesBuilder.new()
  b.write(Bytes.zeroed(4194304))
  for i in 0..1000000 {
    b.put_le((i * 4) % 4194300, i, 4)
  }
  let out = b.freeze()
  var short = BytesBuilder.new()
  short.write_str("abcd")
  short.put_be(0, 0x7879, 2)
  var t = ""
  if let Some(x) = short.freeze().to_str() {
    t = x
  }
  os.stdio.println("${out.len()} ${out[4]} ${out[5]} ${t}")
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	prog := filepath.Join(dir, "prog")
	args := append(append([]string{}, cli[1:]...), "build", "--backend=arm64", "--inline-str", "-o", prog, ".")
	cmd := testtmp.Guard(exec.Command(cli[0], args...))
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build --inline-str: %v\n%s", err, out)
	}
	// linear: milliseconds; the quadratic copy runs for hours
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	out, err := testtmp.Guard(exec.CommandContext(ctx, prog)).CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("a million puts into a 4 MB builder took over 60 s: put copies the builder")
	}
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	// i = 1 wrote bytes 4..7 (the offsets wrap only after 1,048,575 puts)
	if want := "4194304 1 0 xycd\n"; string(out) != want {
		t.Fatalf("got %q, want %q", out, want)
	}
}
