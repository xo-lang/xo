package arm64

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestImageGrayDrawCost guards std/image's draw of an opaque Gray source
// onto RGBA (bench/perf/image gray), which stores a word per two pixels
// with BytesBuilder.put_be(at, w, 8). c0f8e7433 drew in place through
// put_be, then an out of line llrt_rs_put and raw.put per word, and made
// gray 1.842 of its earlier time on the linux/arm64 timing host; the word
// put in line (compiler/ssa call.xo, rt.rs_put_word) brought it to 0.261
// of that (Mac, 200 passes: 9.92G instructions before, 2.68G after; host 1 about 292 ms and 76 ms of user time). The ceilings sit between the two (darwin: /usr/bin/time's
// instruction count, elsewhere user CPU time); IMAGE_GRAY_LOG=1 prints
// the measured value.
func TestImageGrayDrawCost(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	const passes = "200"
	const maxInsn int64 = 5_000_000_000
	const maxUser = 180 * time.Millisecond
	dir, err := filepath.Abs("../../bench/perf/image")
	if err != nil {
		t.Fatal(err)
	}
	res, err := sharedBuild(dir, xotool.Options{Backend: xotool.BackendArm64})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed:\n%s", res.Output)
	}
	bin := res.Binaries[xotool.Host()]
	var cmd *exec.Cmd
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("/usr/bin/time", "-l", bin, "gray", passes)
	} else {
		cmd = exec.Command(bin, "gray", passes)
	}
	cmd = testtmp.Guard(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\n%s", err, stderr.String())
	}
	if runtime.GOOS == "darwin" {
		m := instrRE.FindStringSubmatch(stderr.String())
		if m == nil {
			t.Skipf("no instruction count from /usr/bin/time -l:\n%s", stderr.String())
		}
		n, _ := strconv.ParseInt(m[1], 10, 64)
		if os.Getenv("IMAGE_GRAY_LOG") != "" {
			t.Logf("gray %s passes: %d instructions", passes, n)
		}
		if n > maxInsn {
			t.Errorf("gray %s passes: %d instructions, over the %d ceiling: BytesBuilder.put_be of 8 bytes is no longer in line", passes, n, maxInsn)
		}
		return
	}
	u := cmd.ProcessState.UserTime()
	if os.Getenv("IMAGE_GRAY_LOG") != "" {
		t.Logf("gray %s passes: %v user", passes, u)
	}
	if u > maxUser {
		t.Errorf("gray %s passes: %v of user CPU time, over %v: BytesBuilder.put_be of 8 bytes is no longer in line", passes, u, maxUser)
	}
}
