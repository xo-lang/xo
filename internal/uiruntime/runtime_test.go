// Package uiruntime tests std/ui's browser runtime (RUNTIME_JS in
// std/ui.xo) under Node, as tabs of one browser against a fake sync
// server. The full browser checks (headless Chrome) are run by hand; these
// cover the runtime's own bookkeeping.
package uiruntime

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// runtimeJS extracts RUNTIME_JS from std/ui.xo into a file.
func runtimeJS(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "std", "ui.xo"))
	if err != nil {
		t.Fatal(err)
	}
	const open = "const RUNTIME_JS = r#\"\"\""
	s := string(b)
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal("RUNTIME_JS not found in std/ui.xo")
	}
	s = s[i+len(open):]
	j := strings.Index(s, "\"\"\"#")
	if j < 0 {
		t.Fatal("RUNTIME_JS has no end")
	}
	p := filepath.Join(t.TempDir(), "runtime.js")
	if err := os.WriteFile(p, []byte(s[:j]), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

type report struct {
	Sent    []string `json:"sent"`
	Applied []string `json:"applied"`
	Clients int      `json:"clients"`
}

func run(t *testing.T, scenario string) report {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the runtime runs under Node")
	}
	out, err := exec.Command(node, filepath.Join("testdata", "tabs.mjs"), runtimeJS(t), scenario).CombinedOutput()
	if err != nil {
		t.Fatalf("node: %v\n%s", err, out)
	}
	var r report
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	return r
}

// Two tabs of one browser share localStorage. Each must be its own
// client: with one shared id, the first tab's next Msg reused a seq the
// second tab had sent, and the server dropped it as a duplicate.
func TestTwoTabsAreTwoClients(t *testing.T) {
	r := run(t, "two-tabs")
	if !reflect.DeepEqual(r.Applied, r.Sent) {
		t.Errorf("server applied %q, tabs sent %q", r.Applied, r.Sent)
	}
	if r.Clients != 2 {
		t.Errorf("server saw %d clients, want 2", r.Clients)
	}
}

// Msgs a tab queued offline survive closing it: the next tab adopts the
// closed tab's queue and sends it.
func TestClosedTabQueueIsAdopted(t *testing.T) {
	r := run(t, "reopen")
	if !reflect.DeepEqual(r.Applied, r.Sent) {
		t.Errorf("server applied %q, tabs sent %q", r.Applied, r.Sent)
	}
}
