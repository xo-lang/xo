package diag

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// specSection returns the text of the core.md section that starts with
// heading, up to the next `### ` or `## ` heading.
func specSection(t *testing.T, heading string) string {
	t.Helper()
	data, err := os.ReadFile("../../spec/core.md")
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	i := strings.Index(s, "\n"+heading+"\n")
	if i < 0 {
		t.Fatalf("core.md has no heading %q", heading)
	}
	s = s[i+len(heading)+2:]
	if j := regexp.MustCompile(`\n##+ `).FindStringIndex(s); j != nil {
		s = s[:j[0]]
	}
	return s
}

// TestSpecListsEveryCode keeps the code table of core 10.6 equal to the
// registry in codes.go: every code listed once, with its severity, and
// no code in the spec that the registry does not have.
func TestSpecListsEveryCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	row := regexp.MustCompile("(?m)^\\| `(XO\\d{4})` \\| (\\w+) \\| ([^|]+) \\|")
	listed := map[Code]string{}
	for _, m := range row.FindAllStringSubmatch(sec, -1) {
		c := Code(m[1])
		if _, dup := listed[c]; dup {
			t.Errorf("core 10.6 lists %s twice", c)
		}
		listed[c] = m[2]
	}
	names, codes := codeConstants(t)
	for i, c := range codes {
		sev, ok := listed[c]
		if !ok {
			t.Errorf("%s (%s) is not listed in core 10.6", c, names[i])
			continue
		}
		if sev != string(SeverityOf(c)) {
			t.Errorf("core 10.6 gives %s severity %s, the toolchain reports %s", c, sev, SeverityOf(c))
		}
		delete(listed, c)
	}
	for c := range listed {
		t.Errorf("core 10.6 lists %s, which codes.go does not define", c)
	}
}

// TestSpecListsEveryFaultCode keeps the fault code table of core 10.6
// equal to the `XO-Fnnn` codes the runtimes (std/rt, std/llrt) raise.
func TestSpecListsEveryFaultCode(t *testing.T) {
	sec := specSection(t, "### 10.6 Diagnostic codes")
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile("(?m)^\\| `(XO-F\\d{3})` \\|").FindAllStringSubmatch(sec, -1) {
		listed[m[1]] = true
	}
	used := map[string]bool{}
	fault := regexp.MustCompile(`XO-F\d{3}`)
	for _, dir := range []string{"../../std/rt", "../../std/llrt"} {
		err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "_test.go") || !(strings.HasSuffix(p, ".go") || strings.HasSuffix(p, ".c") || strings.HasSuffix(p, ".h")) {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			for _, c := range fault.FindAllString(string(data), -1) {
				used[c] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(used) == 0 {
		t.Fatal("no fault codes found in the runtimes")
	}
	for c := range used {
		if !listed[c] {
			t.Errorf("the runtime raises %s, which core 10.6 does not list", c)
		}
	}
	for c := range listed {
		if !used[c] {
			t.Errorf("core 10.6 lists %s, which no runtime raises", c)
		}
	}
}
