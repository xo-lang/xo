package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAgentRulesRegistry: every enforcement and test path that
// knowledgebase/infrastructure/agent-rules.md lists exists (task #139,
// owner 2026-11-16: a rule whose hook, check or test went missing drifted
// back to prose unseen). Repo paths always; ~/ paths and workstream: paths
// only on a machine that has the lead directory or the xw repository.
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
	_, wsErr := os.Stat(ws)
	_, leadErr := os.Stat(filepath.Join(home, "xo-lead"))
	for _, r := range rows {
		for _, p := range append(agentRulePaths(r[2]), agentRulePaths(r[3])...) {
			full := ""
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
			}
			if _, err := os.Stat(full); err != nil {
				t.Errorf("agent-rules.md: rule %q: %s is missing (restore it or update the row)", r[0], p)
			}
		}
	}
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
