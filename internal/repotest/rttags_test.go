package repotest

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestStdRTBuildTags builds std/rt under each xo_* build tag its files
// name (found from their //go:build lines), one tag at a time, so a file
// that calls into a tagged file without a stub for the other side fails
// here instead of in every Go-backend build that sets the tag (4b6058367:
// http_client_stream.go called http_net.go's `do`, which xo_nohttp
// builds leave out; gotchas/go-rt-nohttp-tag.md). A few seconds: the
// builds share Go's build cache.
func TestStdRTBuildTags(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(root, "std/rt/*.go"))
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`(?m)^//go:build (.*)$`)
	tag := regexp.MustCompile(`\bxo_[a-z0-9_]+\b`)
	seen := map[string]bool{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range line.FindAllStringSubmatch(string(src), -1) {
			for _, x := range tag.FindAllString(m[1], -1) {
				seen[x] = true
			}
		}
	}
	if !seen["xo_nohttp"] {
		t.Fatal("no //go:build line in std/rt names xo_nohttp: the scan is broken")
	}
	tags := []string{""}
	for x := range seen {
		tags = append(tags, x)
	}
	sort.Strings(tags)
	for _, x := range tags {
		args := []string{"build"}
		if x != "" {
			args = append(args, "-tags", x)
		}
		cmd := exec.Command("go", append(args, "./std/rt")...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("go build -tags %q ./std/rt: %v\n%s", x, err, out)
		}
	}
}
