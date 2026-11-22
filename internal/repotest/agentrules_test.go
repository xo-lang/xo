package repotest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAgentRulesRegistry: every enforcement and test path that
// knowledgebase/infrastructure/agent-rules.md lists exists (task #139,
// owner 2026-11-16: a rule whose hook, check or test went missing drifted
// back to prose unseen). Repo paths always; ~/ paths and workstream: paths
// only on a machine that has the lead directory or the xw repository, and
// only when this tree's registry is the current one (task #659): those
// paths are read from the live machine, so a release branch cut before a
// workstream rename (tier 925 on rel-0551: test/pg_test.sh -> db_test.sh)
// logs them instead of failing.
func TestAgentRulesRegistry(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(root, "knowledgebase", "infrastructure", "agent-rules.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := agentRuleRows(string(data))
	if len(rows) < 10 {
		t.Fatalf("agent-rules.md: %d rule rows, want the table under ## Rules", len(rows))
	}
	home, _ := os.UserHomeDir()
	ws := filepath.Join(home, "godev", "claude-space", "xolang", "workstream")
	live, why := agentRulesLive(root, data)
	errs, warns := checkAgentRules(rows, root, home, ws, live)
	for _, w := range warns {
		t.Logf("%s (not enforced: %s)", w, why)
	}
	for _, e := range errs {
		t.Error(e)
	}
}

// agentRulesLive: whether this tree's agent-rules.md is the current
// registry, so the live machine's ~/ and workstream: paths must match it:
// the tree holds origin/main (main, or a branch on it), or its
// agent-rules.md is origin/main's. Not live without git or origin/main.
func agentRulesLive(dir string, data []byte) (bool, string) {
	if exec.Command("git", "-C", dir, "rev-parse", "--verify", "-q", "refs/remotes/origin/main").Run() != nil {
		return false, "no origin/main in this tree"
	}
	if exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", "refs/remotes/origin/main", "HEAD").Run() == nil {
		return true, ""
	}
	// "./" names the file from dir, whichever directory of the tree it is
	main, err := exec.Command("git", "-C", dir, "show", "refs/remotes/origin/main:./knowledgebase/infrastructure/agent-rules.md").Output()
	if err == nil && string(main) == string(data) {
		return true, ""
	}
	return false, "the tree is not on origin/main and its agent-rules.md differs from main's"
}

// checkAgentRules: the missing paths of rows, as errors, and the missing
// live-machine paths (~/, workstream:) as warnings when live is false.
func checkAgentRules(rows [][]string, root, home, ws string, live bool) (errs, warns []string) {
	_, wsErr := os.Stat(ws)
	_, leadErr := os.Stat(filepath.Join(home, "xo-lead"))
	for _, r := range rows {
		for _, p := range append(agentRulePaths(r[2]), agentRulePaths(r[3])...) {
			full := ""
			machine := true
			switch {
			case strings.HasPrefix(p, "workstream:"):
				if wsErr != nil {
					continue
				}
				full = filepath.Join(ws, strings.TrimPrefix(p, "workstream:"))
			case strings.HasPrefix(p, "~/"):
				if leadErr != nil {
					continue
				}
				full = filepath.Join(home, strings.TrimPrefix(p, "~/"))
			default:
				full = filepath.Join(root, p)
				machine = false
			}
			if _, err := os.Stat(full); err != nil {
				msg := fmt.Sprintf("agent-rules.md: rule %q: %s is missing (restore it or update the row)", r[0], p)
				if machine && !live {
					warns = append(warns, msg)
				} else {
					errs = append(errs, msg)
				}
			}
		}
	}
	return errs, warns
}

// agentRuleRows: the cells of each row of the table under "## Rules"
// (Rule, Home, Enforced by, Test, Incident kinds).
func agentRuleRows(md string) [][]string {
	var out [][]string
	in := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "## ") {
			in = line == "## Rules"
			continue
		}
		if !in || !strings.HasPrefix(line, "| ") || strings.HasPrefix(line, "| Rule |") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), " | ")
		if len(cells) >= 4 {
			out = append(out, cells)
		}
	}
	return out
}

var agentRulePath = regexp.MustCompile("`([^`]+)`")

// agentRulePaths: the backquoted paths of a cell ("none" has none).
func agentRulePaths(cell string) []string {
	var out []string
	for _, m := range agentRulePath.FindAllStringSubmatch(cell, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestAgentRulePaths(t *testing.T) {
	got := agentRulePaths("`a/b.sh` (fn), `workstream:x.xo`")
	if len(got) != 2 || got[0] != "a/b.sh" || got[1] != "workstream:x.xo" {
		t.Errorf("paths: %q", got)
	}
	rows := agentRuleRows("## Rules\n\n| Rule | Home | Enforced by | Test | Incident kinds |\n|---|---|---|---|---|\n| r | h | `x` | none | |\n## Schedules\n| s | x | 1 | `beat:y` |\n")
	if len(rows) != 1 || rows[0][0] != "r" {
		t.Errorf("rows: %q", rows)
	}
}

func TestAgentRulesLive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	rules := filepath.Join(dir, "knowledgebase", "infrastructure", "agent-rules.md")
	write := func(s string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(rules), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rules, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("v1\n")
	git("add", "-A")
	git("commit", "-q", "-m", "base")
	if live, _ := agentRulesLive(dir, []byte("v1\n")); live {
		t.Error("no origin/main: want not live")
	}
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	if live, why := agentRulesLive(dir, []byte("v1\n")); !live {
		t.Errorf("HEAD at origin/main: want live (%s)", why)
	}
	// a release branch: cut at base, main moves on and renames a row
	git("checkout", "-q", "-b", "rel")
	git("commit", "-q", "--allow-empty", "-m", "rel fix")
	git("checkout", "-q", "main")
	write("v2\n")
	git("commit", "-q", "-am", "rename")
	git("update-ref", "refs/remotes/origin/main", "HEAD")
	git("checkout", "-q", "rel")
	if live, _ := agentRulesLive(dir, []byte("v1\n")); live {
		t.Error("release branch with an older registry: want not live")
	}
	if live, why := agentRulesLive(dir, []byte("v2\n")); !live {
		t.Errorf("off main but the same registry as main: want live (%s)", why)
	}
	// a branch on main that edits the registry is live
	git("checkout", "-q", "-b", "feat", "main")
	write("v3\n")
	git("commit", "-q", "-am", "edit rules")
	if live, why := agentRulesLive(dir, []byte("v3\n")); !live {
		t.Errorf("branch on origin/main: want live (%s)", why)
	}
}

func TestCheckAgentRulesWarnsOffMain(t *testing.T) {
	home := t.TempDir()
	ws := filepath.Join(home, "ws")
	repo := t.TempDir()
	for _, d := range []string{ws, filepath.Join(home, "xo-lead"), filepath.Join(repo, "scripts")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	rows := [][]string{
		{"renamed", "h", "`workstream:test/pg_test.sh`", "none"},
		{"lead", "h", "`~/xo-lead/gone.sh`", "none"},
		{"repo", "h", "`scripts/gone.sh`", "none"},
	}
	errs, warns := checkAgentRules(rows, repo, home, ws, false)
	if len(errs) != 1 || !strings.Contains(errs[0], "scripts/gone.sh") {
		t.Errorf("off main errors: %q, want only the repo path", errs)
	}
	if len(warns) != 2 {
		t.Errorf("off main warnings: %q, want the two machine paths", warns)
	}
	errs, warns = checkAgentRules(rows, repo, home, ws, true)
	if len(errs) != 3 || len(warns) != 0 {
		t.Errorf("live: errors %q warnings %q, want 3 errors", errs, warns)
	}
}
