// Package uiruntime tests std/ui's browser runtime (RUNTIME_JS in
// std/ui.xo) under Node, as tabs of one browser against a fake sync
// server, and (nav_test.go) the page layer of decisions 0668 and 0670 on a
// fake DOM. The full browser checks (headless Chrome) are run by hand.
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
	Attrs   []string `json:"attrs"`
	Reloads int      `json:"reloads"`
	Stored  int      `json:"stored"`
	Session int      `json:"session"`
	Before  string   `json:"before"`
	During  string   `json:"during"`
	AfterUp string   `json:"afterUp"`
	After   string   `json:"after"`
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

// A sync answered 401 (decision 0664: no principal) marks the page signed
// out, not offline, and keeps the queue for when the user signs in.
func TestSignedOutKeepsQueue(t *testing.T) {
	r := run(t, "unauthorized")
	if !reflect.DeepEqual(r.Attrs, []string{"data-xo-pending", "data-xo-signed-out"}) {
		t.Errorf("attributes %q, want pending and signed-out only", r.Attrs)
	}
	if r.Reloads != 0 || r.Stored != 1 {
		t.Errorf("reloads %d stored %d, want 0 and the tab's queue", r.Reloads, r.Stored)
	}
}

// A sync answered 403 (the client id belongs to another principal) drops
// the tab's queue and id, so the old principal's Msgs are never sent as
// the new one's, and reloads.
func TestForbiddenDropsQueue(t *testing.T) {
	r := run(t, "forbidden")
	if r.Reloads != 1 || r.Stored != 0 || r.Session != 0 {
		t.Errorf("reloads %d stored %d session %d, want 1, 0, 0", r.Reloads, r.Stored, r.Session)
	}
	if len(r.Applied) != 0 {
		t.Errorf("server applied %q", r.Applied)
	}
}

// A sync reply that lands while a pointer is down on the view does not
// replace it until the pointer is up and the click has run: replacing the
// element under the pointer lost the click (xw-1's dashboard, 2026-11-15).
func TestReplaceWaitsForPointerUp(t *testing.T) {
	r := run(t, "pointer")
	if r.During != r.Before || r.AfterUp != r.Before {
		t.Errorf("view replaced while the pointer was down or before the click ran: before %q, during %q, at pointerup %q", r.Before, r.During, r.AfterUp)
	}
	if r.After == r.Before || r.After != "<p>1</p>" {
		t.Errorf("held view not shown after pointerup: %q", r.After)
	}
}

// TestMorphKeepsNodes runs the runtime's morph over a fake DOM: a new view
// is patched into the live one, so a keyed item, the focused input (and
// what was typed in it), an open <details> and a scrolled box are the same
// nodes afterwards, while text, values the user is not editing, checkboxes
// and changed tags follow the view.
func TestMorphKeepsNodes(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found: the runtime runs under Node")
	}
	out, err := exec.Command(node, filepath.Join("testdata", "morph.mjs"), runtimeJS(t)).CombinedOutput()
	if err != nil {
		t.Fatalf("morph.mjs: %v\n%s", err, out)
	}
	var r map[string]any
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatalf("report: %v\n%s", err, out)
	}
	want := map[string]any{
		"keyedHTML":    `<ul><li data-key="c">C</li><li data-key="a">A!</li><li data-key="d">D</li></ul>`,
		"keyedKept":    true,
		"keyedRemoved": true,
		"focusValue":   "typing",
		"focusKept":    true,
		"otherValue":   "y",
		"text":         "1 items",
		"detailsOpen":  true,
		"scrollKept":   true,
		"boxText":      "y",
		"checked":      true,
		"replacedHTML": `<div><input type="checkbox" name="done" checked=""><b>a</b></div>`,
	}
	for k, w := range want {
		if !reflect.DeepEqual(r[k], w) {
			t.Errorf("%s = %#v, want %#v", k, r[k], w)
		}
	}
}

// A sync reply naming another build (the server was redeployed) marks the
// page stale and reloads it, but only once its queue has been sent: an
// offline page keeps its Msgs until they reach the server.
func TestNewBuildReloadsAfterQueue(t *testing.T) {
	r := run(t, "stale")
	if r.Before != "0,0" || r.Reloads != 1 {
		t.Errorf("reloads before,offline %q after %d, want 0,0 and 1", r.Before, r.Reloads)
	}
	if !reflect.DeepEqual(r.Applied, []string{"a1", "a2"}) {
		t.Errorf("applied %q, want a1 a2", r.Applied)
	}
	if !reflect.DeepEqual(r.Attrs, []string{"data-xo-stale"}) {
		t.Errorf("attributes %q, want data-xo-stale", r.Attrs)
	}
}

// The live socket opening (first, or again after the server restarted)
// syncs at once: before, a page waited for its 30 s check, so changes made
// while the socket was down, or a new build, showed up late.
func TestSocketOpenSyncs(t *testing.T) {
	r := run(t, "reconnect")
	if r.Before != "1" || r.Reloads != 2 {
		t.Errorf("syncs after first open %q, after reopen %d, want 1 and 2", r.Before, r.Reloads)
	}
}

// A Host with redact names the digest of the page each viewer is shown
// (the page's data-xo-seen, then each Sync.seen); the runtime sends back
// the one it has, so the server can answer same while that view did not
// change, however far the version moved (decision 0672's measurement).
func TestSyncSendsSeenDigest(t *testing.T) {
	r := run(t, "seen")
	if r.Before != "d0,d1" {
		t.Errorf("seen digests sent %q, want d0,d1", r.Before)
	}
}

// An App with tab state in the URL renders the view for the page's query:
// every sync sends it (without the "?"), so a sync's html keeps the filter
// the tab is on (decision 0672).
func TestSyncSendsPageQuery(t *testing.T) {
	r := run(t, "query")
	if r.Before != "tag=drink&sort=price" {
		t.Errorf("queries sent %q, want tag=drink&sort=price", r.Before)
	}
}

// Acts on the principal's own state (on_user, decision 0672) go in the
// sync's user part with their own seqs, each once, never as shared Msgs;
// the page's view and state follow the reply; a "u<n>" on the live socket
// syncs only a page whose state is older.
func TestUserActsSyncApart(t *testing.T) {
	r := run(t, "user")
	if r.Before != `{"Type":{"s":"hi"}}|"Post"` {
		t.Errorf("user acts applied %q, want the two acts once each", r.Before)
	}
	if strings.Join(r.Applied, ",") != "a1" {
		t.Errorf("shared Msgs applied %v, want [a1]", r.Applied)
	}
	if r.During != "0,1" {
		t.Errorf("syncs after u2 and u3 %q, want 0,1", r.During)
	}
	if r.After != "<p>1 u2</p>" {
		t.Errorf("view %q, want <p>1 u2</p>", r.After)
	}
	// Type, Post, the shared click (the page has user state), the u3 wake;
	// the second tab starts from the served state and does not sync
	if r.AfterUp != "0,1,2,2" {
		t.Errorf("user versions sent %q, want 0,1,2,2", r.AfterUp)
	}
}

func TestScopedSyncURLKeepsPage(t *testing.T) {
	r := run(t, "scoped")
	if r.Before != "/_xo/sync?page=%2Froom%2F1&thin=1" {
		t.Errorf("sync URL %q, want /_xo/sync?page=%%2Froom%%2F1&thin=1", r.Before)
	}
}

// A wasm client renders its own view: once its module has loaded it asks
// for the model only (view=0), so a reply does not carry the server's
// render as well (28.5 KB at 200 todos in examples/ui-todo, task #451),
// and it shows what its module renders from the reply's model.
func TestWasmSyncSkipsServerView(t *testing.T) {
	r := run(t, "wasm")
	urls := strings.Split(r.Before, ",")
	if last := urls[len(urls)-1]; !strings.Contains(last, "model=1") || !strings.Contains(last, "view=0") {
		t.Errorf("last sync %q, want model=1 and view=0 (all: %q)", last, r.Before)
	}
	if r.After != "<w>1</w>" {
		t.Errorf("view %q, want <w>1</w> (the module's render of the reply's model)", r.After)
	}
}

// A wasm client whose render fails falls back to the server's view: it
// syncs again without view=0 and shows the reply's html.
func TestWasmRenderFailureAsksForView(t *testing.T) {
	r := run(t, "wasm-fail")
	urls := strings.Split(r.Before, ",")
	if last := urls[len(urls)-1]; strings.Contains(last, "view=0") || !strings.Contains(last, "model=1") {
		t.Errorf("last sync %q, want model=1 without view=0 (all: %q)", last, r.Before)
	}
	if r.After != "<p>1</p>" {
		t.Errorf("view %q, want the server's <p>1</p>", r.After)
	}
}
