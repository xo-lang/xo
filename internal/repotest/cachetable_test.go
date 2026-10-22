package repotest

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestCacheTable: every directory the source names under <user cache>/xo
// is an entry of compiler/prune/clean.xo's caches(), the table `xo clean
// -cache` empties, so a new cache cannot be missed. The forms looked for:
// `${...cache...}/xo/NAME` and `"/xo/NAME"` in .xo files, and
// filepath.Join(..., "xo", "NAME") or the xotool cache base joined with
// "NAME" in .go files.
func TestCacheTable(t *testing.T) {
	table, err := os.ReadFile(filepath.Join(root, "compiler", "prune", "clean.xo"))
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, m := range regexp.MustCompile(`Cache\{name: "([a-z0-9-]+)"`).FindAllStringSubmatch(string(table), -1) {
		have[m[1]] = true
	}
	if len(have) < 10 {
		t.Fatalf("found %d entries in clean.xo", len(have))
	}
	xoForms := []*regexp.Regexp{
		regexp.MustCompile(`[cC]ache[^"\n]*\}/xo/([a-z0-9-]+)`),
		regexp.MustCompile(`"/xo/([a-z0-9-]+)`),
	}
	goForms := []*regexp.Regexp{
		regexp.MustCompile(`"xo", "([a-z0-9-]+)"`),
		regexp.MustCompile(`Join\(base, "([a-z0-9-]+)"`),
	}
	found := map[string]string{}
	scan := func(dir, ext string, forms []*regexp.Regexp) {
		filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ext) || strings.HasSuffix(p, "_test"+ext) || strings.Contains(p, "/testdata/") {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for _, re := range forms {
				for _, m := range re.FindAllStringSubmatch(string(b), -1) {
					if _, ok := found[m[1]]; !ok {
						found[m[1]] = strings.TrimPrefix(p, root+"/")
					}
				}
			}
			return nil
		})
	}
	scan("compiler", ".xo", xoForms)
	scan("internal", ".go", goForms)
	scan("cmd", ".go", goForms)
	if len(found) < 5 {
		t.Fatalf("found only %d cache names in the source: the patterns no longer match", len(found))
	}
	// matches that are not caches under <user cache>/xo
	notCache := map[string]bool{
		"seed-bucket": true, // <user config>/xo/seed-bucket (xotool.SeedBucketFile)
		"bin":         true, // seeds/V/PLAT/tree/xo/bin: inside a seed's tree
		"xo":          true, // a tree's xo/ directory, not a cache
	}
	for name, file := range found {
		if !have[name] && !notCache[name] {
			t.Errorf("%s names the cache <user cache>/xo/%s, which compiler/prune/clean.xo's caches() lacks: add it (xo clean -cache)", file, name)
		}
	}
}
