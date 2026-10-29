package xotool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestWindowsLinkLibs: the import libraries compileC adds on a windows
// target are the ones compiler/xo link.xo's sys_link_args gives xo's own
// link (one list in two languages; CI 40926030751: the test harness linked
// without synchronization.lib and the futex did not resolve).
func TestWindowsLinkLibs(t *testing.T) {
	root, err := Root()
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(root, "compiler", "xo", "link.xo"))
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(src), "fn sys_link_args(")
	if i < 0 {
		t.Fatal("compiler/xo/link.xo: no sys_link_args")
	}
	body := string(src)[i:]
	// the windows branch's list: its return line
	if j := strings.Index(body, "\"-fuse-ld=lld\""); j > 0 {
		body = body[j:]
	} else {
		t.Fatal("compiler/xo/link.xo sys_link_args: no -fuse-ld=lld line")
	}
	if j := strings.Index(body, "\n"); j > 0 {
		body = body[:j]
	}
	var got []string
	for _, m := range regexp.MustCompile(`"-l([a-z0-9_]+)"`).FindAllStringSubmatch(body, -1) {
		got = append(got, m[1])
	}
	want := strings.Join(WindowsLinkLibs, " ")
	if strings.Join(got, " ") != want {
		t.Fatalf("link.xo sys_link_args links %q, xotool.WindowsLinkLibs is %q: keep the two lists equal", strings.Join(got, " "), want)
	}
}
