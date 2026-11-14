package repotest

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var changelogVersion = regexp.MustCompile(`^## (v\d+\.\d+\.\d+)\b`)

// changelogSections maps each `## vX.Y.Z` heading of a CHANGELOG to its body
// (the lines up to the next `## ` heading), trimmed.
func changelogSections(text string) map[string]string {
	secs := map[string]string{}
	cur := ""
	var body []string
	flush := func() {
		if cur != "" {
			secs[cur] = strings.TrimSpace(strings.Join(body, "\n"))
		}
	}
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "## ") {
			flush()
			cur, body = "", nil
			if m := changelogVersion.FindStringSubmatch(l); m != nil {
				cur = m[1]
			}
			continue
		}
		if cur != "" {
			body = append(body, l)
		}
	}
	flush()
	return secs
}

// changelogEditedBefore pins released sections that already differed from their
// tag when this test was added (2026-11-13): older merge backs misfiled lines
// (v0.38.1 and v0.33.1 lost lines; v0.39.0's tag files decision 0488's line
// under another heading). The value is the SHA-256 of main's section, so any
// further edit of these fails too.
var changelogEditedBefore = map[string]string{
	"v0.39.0": "f44d13df9e25c2c1ce7117af5fd7677c785d5b36d83663be56695cf3260ed844",
	"v0.38.1": "e3dc8b83e074165f50b7eb322f6a2edc0f24b05b561dfeed0d108aaf10362bc0",
	"v0.33.1": "e9a2df92f29c82d116971ba648d631232109daed9fb8dad1aaaf8bd4c85cf632",
}

// TestChangelogReleasedSectionsUnchanged: a released `## vX.Y.Z` section of
// CHANGELOG.md is the text that shipped, so it equals the section in the
// tag's own CHANGELOG.md. On 2026-11-13 a post-cut commit (c50c071c1) rewrote a
// v0.51.0 line in place to describe a default that 0.51 does not ship; the
// 0.51.0 merge back restored it. A later change goes under Unreleased. Tags
// whose CHANGELOG has no section of their own (before v0.36) are skipped, and
// so is a tag the checkout does not have.
func TestChangelogReleasedSectionsUnchanged(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for v, body := range changelogSections(string(data)) {
		out, err := exec.Command("git", "-C", root, "show", v+":CHANGELOG.md").Output()
		if err != nil {
			continue // no such tag here
		}
		at, ok := changelogSections(string(out))[v]
		if !ok {
			continue
		}
		if want, pinned := changelogEditedBefore[v]; pinned {
			sum := sha256.Sum256([]byte(body))
			if got := hex.EncodeToString(sum[:]); got != want {
				t.Errorf("CHANGELOG.md %s: the released section changed (sha256 %s, pinned %s); released sections are the text that shipped, add the change under Unreleased", v, got, want)
			}
			continue
		}
		checked++
		if at != body {
			a := strings.Split(at, "\n")
			b := strings.Split(body, "\n")
			inA := map[string]bool{}
			for _, l := range a {
				inA[l] = true
			}
			inB := map[string]bool{}
			for _, l := range b {
				inB[l] = true
			}
			for _, l := range a {
				if !inB[l] {
					t.Errorf("CHANGELOG.md %s: a line of the release is gone or changed: %.120s", v, l)
				}
			}
			for _, l := range b {
				if !inA[l] {
					t.Errorf("CHANGELOG.md %s: a line the release did not have (put it under Unreleased): %.120s", v, l)
				}
			}
		}
	}
	if checked == 0 {
		t.Skip("no release tags in this checkout")
	}
}
